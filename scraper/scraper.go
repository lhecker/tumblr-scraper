package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
	"github.com/lhecker/tumblr-scraper/semaphore"
)

var (
	errFileNotFound = errors.New("file not found")

	videoURLFixupRegexp  = regexp.MustCompile(`_(?:480|720)\.mp4$`)
	imageSizeFixupRegexp = regexp.MustCompile(`_(?:\d+)\.([a-z]+)$`)
	inlineMediaRegexp    = regexp.MustCompile(`http[^"]+(?:media|vtt)\.tumblr\.com/[^"]+`)
	skippableURLs        = regexp.MustCompile(`media\.tumblr\.com/avatar_`)
)

func init() {
	for _, e := range []struct{ typ, ext string }{
		{"image/bmp", ".bmp"},
		{"image/gif", ".gif"},
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/tiff", ".tiff"},
		{"image/webp", ".webp"},
		{"video/webm", ".webm"},
	} {
		err := mime.AddExtensionType(e.ext, e.typ)
		if err != nil {
			panic(err)
		}
	}
}

type Scraper struct {
	client   *http.Client
	config   *config.Config
	database *database.Database
}

func NewScraper(client *http.Client, config *config.Config, database *database.Database) *Scraper {
	return &Scraper{
		client:   client,
		config:   config,
		database: database,
	}
}

func (s *Scraper) Scrape(ctx context.Context, blogConfig *config.BlogConfig) (int64, error) {
	err := os.MkdirAll(blogConfig.Target, 0755)
	if err != nil {
		return 0, err
	}

	eg, ctx := errgroup.WithContext(ctx)
	sc := &scrapeContext{
		scraper:    s,
		blogConfig: blogConfig,
		errgroup:   eg,
		ctx:        ctx,

		state: scrapeContextStateTryUseAPI,

		lowestID:  math.MaxInt64,
		highestID: math.MinInt64,

		sema: semaphore.NewPrioritySemaphore(s.config.Concurrency),
	}
	if !blogConfig.Rescrape {
		sc.highestID, err = s.database.GetHighestID(blogConfig.Name)
		if err != nil {
			return 0, err
		}
	}
	if !blogConfig.Before.IsZero() {
		sc.before = blogConfig.Before
	}

	err = sc.Scrape()
	if err != nil {
		return 0, err
	}

	return sc.highestID, nil
}

type scrapeContextState int

const (
	scrapeContextStateTryUseAPI scrapeContextState = iota
	scrapeContextStateUseAPI
	scrapeContextStateUseIndashAPI
)

type scrapeContext struct {
	// Structurized arguments
	scraper    *Scraper
	blogConfig *config.BlogConfig
	errgroup   *errgroup.Group
	ctx        context.Context

	// General state of this scrapeContext
	state scrapeContextState

	// Current pagination state
	offset int
	before time.Time

	// Informational values
	lowestID  int64
	highestID int64

	// Other private members
	sema *semaphore.PrioritySemaphore
}

func (sc *scrapeContext) Scrape() (err error) {
	log.Printf("%s: scraping starting at %d", sc.blogConfig.Name, sc.highestID)
	defer func() { log.Printf("%s: scraping finished at %d", sc.blogConfig.Name, sc.highestID) }()

	defer func() {
		e := sc.errgroup.Wait()
		if err == nil {
			err = e
		}
	}()

	initialHighestID := sc.highestID

	for {
		if sc.before.IsZero() {
			log.Printf("%s: fetching posts", sc.blogConfig.Name)
		} else {
			log.Printf("%s: fetching posts before %s", sc.blogConfig.Name, sc.before.Format("2006-01-02T15:04:05Z"))
		}

		var data *postsResponse
		data, err = sc.scrapeBlog()
		if err != nil {
			return
		}

		// Entries returned by Tumblr's paginated API can overlap between pages.
		// I.e. specifying `&before=1491103082` might still randomly return entries with exactly such a timestamp.
		// => Filter out redundant entries with post IDs we already scraped in previous iterations.
		posts := []*post(nil)
		for idx, post := range data.Response.Posts {
			if post.ID < sc.lowestID {
				posts = data.Response.Posts[idx:]
				break
			}
		}

		if len(posts) == 0 {
			return
		}

		for _, post := range posts {
			if post.ID < sc.lowestID {
				sc.lowestID = post.ID
			}
			if post.ID > sc.highestID {
				sc.highestID = post.ID
			}

			timestamp := post.timestamp()
			if sc.before.IsZero() || timestamp.Before(sc.before) {
				sc.before = timestamp
			}

			if post.ID <= initialHighestID {
				return
			}

			if sc.blogConfig.IgnoreReblogs && len(post.RebloggedFromID) != 0 {
				continue
			}

			sc.scrapePost(post)
		}

		sc.offset += len(data.Response.Posts)
	}
}

func (sc *scrapeContext) scrapeBlog() (data *postsResponse, err error) {
	for data == nil {
		data, err = sc.scrapeBlogMaybe()
		if err != nil {
			return
		}
	}
	return
}

func (sc *scrapeContext) scrapeBlogMaybe() (*postsResponse, error) {
	sc.sema.Acquire(sc.offset)
	defer sc.sema.Release()

	var (
		url *url.URL
		res *http.Response
		err error
	)

	switch sc.state {
	case scrapeContextStateUseIndashAPI:
		url = sc.getIndashBlogPostsURL()
		res, err = sc.doGetRequest(url, http.Header{
			"Referer":          {"https://www.tumblr.com/dashboard"},
			"X-Requested-With": {"XMLHttpRequest"},
		})
	default:
		url = sc.getAPIPostsURL()
		res, err = sc.doGetRequest(url, nil)
	}

	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch {
	case
		res.StatusCode == http.StatusNotFound &&
			sc.state == scrapeContextStateTryUseAPI &&
			len(sc.scraper.config.Username) != 0:
		// Retry using the indash_blog API (i.e. for non-public blogs)
		err := account.LoginOnce()
		if err != nil {
			return nil, err
		}

		sc.state = scrapeContextStateUseIndashAPI
		return nil, nil
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GET %s failed with: %d %s", url, res.StatusCode, res.Status)
	}

	data := &postsResponse{}
	err = json.NewDecoder(res.Body).Decode(data)
	if err != nil {
		return nil, err
	}

	if sc.state == scrapeContextStateTryUseAPI {
		sc.state = scrapeContextStateUseAPI
	}

	return data, nil
}

func (sc *scrapeContext) scrapePost(post *post) {
	for _, text := range []string{post.PostHTML, post.Body, post.Answer} {
		if len(text) == 0 {
			continue
		}

		for _, u := range inlineMediaRegexp.FindAllString(text, -1) {
			if !sc.shouldURLBeSkipped(u) {
				sc.downloadFileAsync(post, u)
			}
		}
	}

	for _, photo := range post.Photos {
		sc.downloadFileAsync(post, photo.OriginalSize.URL)
	}

	if len(post.VideoURL) != 0 {
		sc.downloadFileAsync(post, post.VideoURL)
	}
}

func (sc *scrapeContext) downloadFileAsync(post *post, rawurl string) {
	if len(rawurl) == 0 {
		panic("missing url")
	}

	// Until the Goroutine below is executed, sc.offset might've already been incremented.
	// => Create a snapshot here.
	priority := sc.offset

	sc.errgroup.Go(func() error {
		return sc.downloadFile(post, rawurl, priority)
	})
}

func (sc *scrapeContext) downloadFile(post *post, rawurl string, priority int) error {
	optimalRawurl := sc.fixupURL(rawurl)

	// First try to download the optimal URL (i.e. the highest resolution)
	// and fall back to the original URL if that fails with a 404 error.
	err := sc.downloadFileMaybe(post, optimalRawurl, priority)
	if err == errFileNotFound && optimalRawurl != rawurl {
		err = sc.downloadFileMaybe(post, rawurl, priority)
	}

	// Ignore 404 errors
	if err == errFileNotFound {
		err = nil
	}

	return err
}

func (sc *scrapeContext) downloadFileMaybe(post *post, rawurl string, priority int) error {
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}

	sc.sema.Acquire(priority)
	defer sc.sema.Release()

	path := filepath.Join(sc.blogConfig.Target, filepath.Base(rawurl))
	fileTime := post.timestamp()

	// File already exists --> nothing to do here.
	_, err = os.Lstat(path)
	if err == nil {
		log.Printf("%s: skipping %s", sc.blogConfig.Name, path)
		return nil
	}

	res, err := sc.doGetRequest(u, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		// continue below
	case http.StatusForbidden:
		// If a video or image was fully/entirely deleted (e.g. due to DMCA) it will
		// still be linked inside the posts but result in a "403 Forbidden" error.
		return nil
	case http.StatusNotFound:
		log.Printf("%s: did not find %s", sc.blogConfig.Name, path)
		return errFileNotFound
	default:
		return fmt.Errorf("GET %s failed with: %d %s", rawurl, res.StatusCode, res.Status)
	}

	fixedPath := sc.fixupFilepath(res, path)
	if fixedPath != path {
		path = fixedPath

		// Same as above: File already exists --> nothing to do here.
		_, err = os.Lstat(path)
		if err == nil {
			log.Printf("%s: skipping %s", sc.blogConfig.Name, path)
			return nil
		}
	}

	// Prevent concurrent writes into the temporary file.
	// A blog can contain the same image link multiple times.
	// If such a duplicate link is encountered while we're still writing into the .tmp-file,
	// this will corrupt the original content and make the os.Rename() operation fail spuriously.
	tempPath := path + ".tmp"
	if !acquireTempFile(tempPath) {
		return nil
	}
	defer releaseTempFile(tempPath)

	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, res.Body)
	if err != nil {
		return err
	}

	err = tempFile.Close()
	tempFile = nil
	if err != nil {
		return err
	}

	err = os.Chtimes(tempPath, fileTime, fileTime)
	if err != nil {
		return err
	}

	err = os.Rename(tempPath, path)
	if err != nil {
		return err
	}

	log.Printf("%s: wrote %s", sc.blogConfig.Name, path)
	return nil
}

func (sc *scrapeContext) getAPIPostsURL() *url.URL {
	u, err := url.Parse(fmt.Sprintf("https://api.tumblr.com/v2/blog/%s/posts", sc.blogConfig.Name))
	if err != nil {
		panic(err)
	}

	vals := url.Values{
		"api_key": {sc.scraper.config.APIKey},
		"limit":   {"20"},
	}
	if sc.blogConfig.IgnoreReblogs {
		vals.Set("reblog_info", "1")
	}
	if !sc.before.IsZero() {
		vals.Set("before", strconv.FormatInt(sc.before.Unix(), 10))
	}
	u.RawQuery = vals.Encode()

	return u
}

func (sc *scrapeContext) getIndashBlogPostsURL() *url.URL {
	u, err := url.Parse("https://www.tumblr.com/svc/indash_blog")
	if err != nil {
		panic(err)
	}

	u.RawQuery = url.Values{
		"tumblelog_name_or_id": {strings.TrimSuffix(sc.blogConfig.Name, ".tumblr.com")},
		"post_id":              {},
		"limit":                {"20"},
		"offset":               {strconv.Itoa(sc.offset)},
	}.Encode()

	return u
}

func (sc *scrapeContext) doGetRequest(url *url.URL, header http.Header) (*http.Response, error) {
	if header == nil {
		header = make(http.Header)
	}

	req := &http.Request{
		Method: http.MethodGet,
		URL:    url,
		Header: header,
	}
	req = req.WithContext(sc.ctx)
	return sc.scraper.client.Do(req)
}

// There seems to be a migration fuckup on Tumblr's side for posts pre-2014,
// where some inline images are replaced with the quoted poster's avatar.
// --> Filter those out.
func (sc *scrapeContext) shouldURLBeSkipped(url string) bool {
	return skippableURLs.MatchString(url)
}

func (sc *scrapeContext) fixupURL(url string) string {
	if strings.HasSuffix(url, ".mp4") {
		return videoURLFixupRegexp.ReplaceAllString(url, ".mp4")
	}

	return imageSizeFixupRegexp.ReplaceAllString(url, "_1280.$1")
}

// Tumblr suffixes some files with an invalid extension, like .gifv for instance.
// The response then includes an Content-Disposition header with the actual, supposed "filename".
// Furthermore a Content-Type header is sent with a MIME type which we use as a fallback.
func (sc *scrapeContext) fixupFilepath(res *http.Response, path string) string {
	// The Content-Disposition header can include a "filename" a browser is supposed to use to name the downloaded file.
	_, contentDispositionParams, _ := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
	if contentDispositionParams != nil {
		filename := contentDispositionParams["filename"]
		if len(filename) != 0 {
			return filepath.Join(sc.blogConfig.Target, filename)
		}
	}

	exts, _ := mime.ExtensionsByType(res.Header.Get("Content-Type"))
	if len(exts) != 0 {
		dir, file := filepath.Split(path)
		curExt := filepath.Ext(file)

		for _, ext := range exts {
			if ext == curExt {
				// There's nothing we need to do if one of the extensions suggested
				// by the Content-Type already matches what we use for "path".
				return path
			}
		}

		basename := strings.TrimSuffix(file, curExt)
		file = basename + exts[0]
		return filepath.Join(dir, file)
	}

	return path
}
