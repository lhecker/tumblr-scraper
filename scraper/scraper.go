package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/sync/errgroup"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
	"github.com/lhecker/tumblr-scraper/semaphore"
)

var (
	errFileNotFound = errors.New("file not found")

	deactivatedNameSuffixLength = 20
	deactivatedNameRegexp       = regexp.MustCompile(`.-deactivated\d{8}$`)

	videoURLFixupRegexp  = regexp.MustCompile(`_(?:480|720)\.mp4$`)
	imageSizeFixupRegexp = regexp.MustCompile(`_(?:\d+)\.([a-z]+)$`)

	mediaURLRegexp     = regexp.MustCompile(`^http.+(?:media|vtt)\.tumblr\.com/.+$`)
	htmlMediaURLRegexp = regexp.MustCompile(`http[^"]+(?:media|vtt)\.tumblr\.com/[^"]+`)
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

	sc, err := newScrapeContext(s, blogConfig, eg, ctx)
	if err != nil {
		return 0, err
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
	scrapeContextStateTryUseIndashAPI
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
	sema         *semaphore.PrioritySemaphore
	allowedBlogs map[string]struct{}
}

func newScrapeContext(
	s *Scraper,
	blogConfig *config.BlogConfig,
	eg *errgroup.Group,
	ctx context.Context,
) (*scrapeContext, error) {
	var err error

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
			return nil, err
		}
	}

	if !blogConfig.Before.IsZero() {
		sc.before = blogConfig.Before
	}

	if blogConfig.AllowReblogsFrom != nil {
		sc.allowedBlogs = map[string]struct{}{
			blogConfig.Name: {},
		}

		for _, from := range *blogConfig.AllowReblogsFrom {
			sc.allowedBlogs[from] = struct{}{}
		}
	}

	return sc, nil
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

		var res *postsResponse
		res, err = sc.scrapeBlog()
		if err != nil {
			return
		}
		if len(res.Response.Posts) == 0 {
			return
		}

		for _, post := range res.Response.Posts {
			post.id, err = post.ID.Int64()
			if err != nil {
				return
			}
		}

		for _, post := range res.Response.Posts {
			if post.id < sc.lowestID {
				sc.lowestID = post.id
			}
			if post.id > sc.highestID {
				sc.highestID = post.id
			}

			timestamp := post.timestamp()
			if sc.before.IsZero() || timestamp.Before(sc.before) {
				sc.before = timestamp
			}

			if post.id <= initialHighestID {
				return
			}

			if !sc.handleReblogs(post) {
				continue
			}

			err = sc.scrapePost(post)
			if err != nil {
				return
			}
		}

		sc.offset += len(res.Response.Posts)
	}
}

// Returns true if the post is ok to be scraped
func (sc *scrapeContext) handleReblogs(post *post) bool {
	if sc.allowedBlogs == nil {
		return true
	}

	// In case a blog is renamed from A -> B, then
	//   .Trail[0].Blog.Name == "A"
	// (with .Trail[0] being the .IsRootItem), but
	//   .RebloggedRootName == "B"
	//
	// => *Always* use .RebloggedRootName as primary means to decide whether this is really-really a reblog.
	// => Sometimes when the root blog is deleted only .RebloggedFromName is defined, so use it as a fallback.

	if len(post.RebloggedRootName) != 0 {
		return sc.handleReblogsUsingName(post, post.RebloggedRootName)
	}
	if isAllowed, ok := sc.handleReblogsUsingTrail(post); ok {
		return isAllowed
	}
	if len(post.RebloggedFromName) != 0 {
		return sc.handleReblogsUsingName(post, post.RebloggedFromName)
	}

	return true
}

func (sc *scrapeContext) handleReblogsUsingName(post *post, name string) bool {
	if deactivatedNameRegexp.MatchString(name) {
		name = name[0 : len(name)-deactivatedNameSuffixLength]
	}

	if !sc.isBlogAllowed(name) {
		return false
	}

	sc.filterReblogsFromBody(post)
	return true
}

func (sc *scrapeContext) handleReblogsUsingTrail(post *post) (bool, bool) {
	var root *trailEntry
	for _, entry := range post.Trail {
		if entry.IsRootItem == nil || *entry.IsRootItem {
			root = &entry
			break
		}
	}
	if root == nil {
		return false, false
	}
	if !sc.isBlogAllowed(config.TumblrNameToDomain(root.Blog.Name)) {
		return false, true
	}

	sc.filterReblogsFromBody(post)
	return true, true
}

// Patches the post body to only include posts from our bloggers of interest
func (sc *scrapeContext) filterReblogsFromBody(post *post) {
	if len(post.Trail) < 2 {
		return
	}

	body := strings.Builder{}

	for _, entry := range post.Trail {
		if !sc.isBlogAllowed(config.TumblrNameToDomain(entry.Blog.Name)) {
			continue
		}
		if len(entry.ContentRaw) == 0 {
			return
		}
		body.WriteString(entry.ContentRaw)
	}

	post.Body = body.String()
}

func (sc *scrapeContext) isBlogAllowed(name string) bool {
	_, ok := sc.allowedBlogs[config.TumblrNameToDomain(name)]
	return ok
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
	case scrapeContextStateTryUseIndashAPI, scrapeContextStateUseIndashAPI:
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

	if res.StatusCode != http.StatusOK {
		if sc.state == scrapeContextStateTryUseAPI && res.StatusCode == http.StatusNotFound && len(sc.scraper.config.Username) != 0 {
			sc.state = scrapeContextStateTryUseIndashAPI
			return nil, nil
		}
		if sc.state == scrapeContextStateTryUseIndashAPI && res.StatusCode != http.StatusNotFound {
			err := account.LoginOnce()
			if err != nil {
				return nil, err
			}
			sc.state = scrapeContextStateUseIndashAPI
			return nil, nil
		}
		return nil, fmt.Errorf("GET %s failed with: %d %s", url, res.StatusCode, res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	data := &postsResponse{}
	err = json.Unmarshal(body, data)
	if err != nil {
		return nil, err
	}

	if sc.state == scrapeContextStateTryUseAPI {
		sc.state = scrapeContextStateUseAPI
	}

	return data, nil
}

func (sc *scrapeContext) scrapePost(post *post) error {
	//
	// Scraping logic for NPF posts
	//

	err := sc.scrapeNpfContent(post, post.Content)
	if err != nil {
		return err
	}

	for _, t := range post.Trail {
		name := t.BrokenBlogName
		if len(t.Blog.Name) != 0 {
			name = t.Blog.Name
		}

		if !sc.isBlogAllowed(name) {
			continue
		}

		var cs []content
		err = json.Unmarshal(t.Content, &cs)
		if err != nil {
			continue
		}

		err = sc.scrapeNpfContent(post, cs)
		if err != nil {
			return err
		}
	}

	//
	// Scraping logic for indash posts
	//

	bodyScraped := false
	for _, text := range []string{post.Body, post.Answer} {
		if len(text) != 0 {
			bodyScraped = true
			sc.scrapePostBody(post, text)
		}
	}
	if !bodyScraped && len(post.Reblog.Comment) != 0 {
		sc.scrapePostBody(post, post.Reblog.Comment)
	}

	for _, photo := range post.Photos {
		sc.downloadFileAsync(post, photo.OriginalSize.URL)
	}
	if len(post.VideoURL) != 0 {
		sc.downloadFileAsync(post, post.VideoURL)
	}

	return nil
}

func (sc *scrapeContext) scrapeNpfContent(post *post, cs []content) error {
	for _, c := range cs {
		if len(c.Media) == 0 {
			continue
		}

		switch c.Type {
		case "image":
			var ms imageMedia
			err := json.Unmarshal(c.Media, &ms)
			if err != nil {
				return err
			}

			bestURL := ms[0].URL
			bestArea := ms[0].Width * ms[0].Height

			for _, m := range ms {
				if m.HasOriginalDimensions {
					bestURL = m.URL
					break
				}
				if m.Width*m.Height > bestArea {
					bestURL = m.URL
				}
			}

			sc.downloadFileAsync(post, bestURL)
		case "video":
			var ms videoMedia
			err := json.Unmarshal(c.Media, &ms)
			if err != nil {
				return err
			}

			if strings.Contains(ms.URL, "tumblr.com") {
				sc.downloadFileAsync(post, ms.URL)
			}
		}
	}

	return nil
}

func (sc *scrapeContext) scrapePostBody(post *post, text string) {
	nodes, err := html.ParseFragment(strings.NewReader(text), &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Div,
		Data:     "div",
	})
	if err != nil {
		log.Printf("%s: failed to parse body - falling back to regexp: %v", sc.blogConfig.Name, err)
		sc.scrapePostBodyUsingSearch(post, text)
		return
	}

	for len(nodes) != 0 {
		idx := len(nodes) - 1

		node := nodes[idx]
		nodes[idx] = nil

		nodes = nodes[0:idx]

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			nodes = append(nodes, child)
		}

		if node.Type != html.ElementNode {
			continue
		}

		for _, attr := range node.Attr {
			switch attr.Key {
			case "href", "src", "data-big-photo":
				if mediaURLRegexp.MatchString(attr.Val) {
					sc.downloadFileAsync(post, attr.Val)
				}
			}
		}
	}
}

func (sc *scrapeContext) scrapePostBodyUsingSearch(post *post, text string) {
	for _, u := range htmlMediaURLRegexp.FindAllString(text, -1) {
		sc.downloadFileAsync(post, u)
	}
}

func (sc *scrapeContext) downloadFileAsync(post *post, rawurl string) {
	if len(rawurl) == 0 {
		panic("missing url")
	}

	sc.sema.Acquire(sc.offset)
	sc.errgroup.Go(func() error {
		defer sc.sema.Release()
		return sc.downloadFile(post, rawurl)
	})
}

func (sc *scrapeContext) downloadFile(post *post, rawurl string) error {
	optimalRawurl := sc.fixupURL(rawurl)

	// First try to download the optimal URL (i.e. the highest resolution)
	// and fall back to the original URL if that fails with a 404 error.
	err := sc.downloadFileMaybe(post, optimalRawurl)
	if err == errFileNotFound && optimalRawurl != rawurl {
		err = sc.downloadFileMaybe(post, rawurl)
	}

	// Ignore 404 errors
	if err == errFileNotFound {
		log.Printf("%s: did not find %s", sc.blogConfig.Name, rawurl)
		err = nil
	}

	if err != nil {
		log.Printf("%s: failed to download file: %v", sc.blogConfig.Name, err)
	}
	return err
}

func (sc *scrapeContext) downloadFileMaybe(post *post, rawurl string) error {
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}

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
		return errFileNotFound
	case http.StatusInternalServerError:
		return errFileNotFound
	default:
		return fmt.Errorf("GET %s failed with: %d %s", rawurl, res.StatusCode, res.Status)
	}

	lastModifiedString := res.Header.Get("Last-Modified")
	if len(lastModifiedString) != 0 {
		lastModified, err := time.Parse(time.RFC1123, lastModifiedString)
		if err != nil {
			log.Printf("%s: failed to parse Last-Modified header: %v", sc.blogConfig.Name, err)
		} else if fileTime.Sub(lastModified) > 24*time.Hour {
			fileTime = lastModified
		}
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

	if !acquireFile(path) {
		return nil
	}
	defer releaseFile(path)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, res.Body)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}

	err = file.Close()
	if err != nil {
		_ = os.Remove(path)
		return err
	}

	err = os.Chtimes(path, fileTime, fileTime)
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
		"npf":     {"true"},
	}
	if sc.allowedBlogs != nil {
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
		"tumblelog_name_or_id":           {config.TumblrDomainToName(sc.blogConfig.Name)},
		"post_id":                        {""},
		"limit":                          {"20"},
		"offset":                         {strconv.Itoa(sc.offset)},
		"should_bypass_safemode_forpost": {"true"},
		"should_bypass_safemode_forblog": {"true"},
		"should_bypass_tagfiltering":     {"true"},
		"can_modify_safe_mode":           {"true"},
		"npf":                            {"true"},
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
