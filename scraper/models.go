package scraper

import (
	"encoding/json"
	"time"
)

type postsResponse struct {
	Response struct {
		Posts []*post `json:"posts"`
	} `json:"response"`
}

type post struct {
	ID json.Number `json:"id"`
	id int64

	Timestamp int64        `json:"timestamp"`
	Trail     []trailEntry `json:"trail"`

	// NPF content: https://www.tumblr.com/docs/npf
	Content    []content `json:"content"`
	Layout     []layout  `json:"layout"`
	PostAuthor string    `json:"post_author"`

	// Compatibility with the private API used by Tumblr's Dashboard
	Body     string  `json:"body"`
	Photos   []photo `json:"photos"`
	VideoURL string  `json:"video_url"`
	Answer   string  `json:"answer"`

	// Only defined for reblogs
	RebloggedFromName string `json:"reblogged_from_name"`
	RebloggedRootName string `json:"reblogged_root_name"`
	Reblog            reblog `json:"reblog"`
}

func (s *post) timestamp() time.Time {
	return time.Unix(s.Timestamp, 0)
}

type photo struct {
	OriginalSize photoVariant `json:"original_size"`
}

type photoVariant struct {
	URL string `json:"url"`
}

type reblog struct {
	Comment string `json:"comment"`
}

type trailEntry struct {
	Blog struct {
		Name string `json:"name"`
	} `json:"blog"`
	BrokenBlogName string `json:"broken_blog_name"`

	// NPF content: https://www.tumblr.com/docs/npf
	Content []content `json:"content"`
	Layout  []layout  `json:"layout"`

	ContentRaw string `json:"content_raw"`
	IsRootItem *bool  `json:"is_root_item"`
}

type content struct {
	Type  string          `json:"type"`
	Media json.RawMessage `json:"media"`
}

type imageMedia []struct {
	URL                   string `json:"url"`
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	HasOriginalDimensions bool   `json:"has_original_dimensions"`
}

type videoMedia struct {
	URL string `json:"url"`
}

type layout struct {
	Type        string `json:"type"`
	Blocks      []int  `json:"blocks"`
	Attribution struct {
		URL string `json:"url"`
	} `json:"attribution"`
}
