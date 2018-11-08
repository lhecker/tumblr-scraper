package scraper

import (
	"time"
)

type postsResponse struct {
	Response struct {
		Posts []*post `json:"posts"`
	} `json:"response"`
}

type post struct {
	ID        int64        `json:"id"`
	Timestamp int64        `json:"timestamp"`
	Trail     []trailEntry `json:"trail"`
	Content   []content    `json:"content"`

	// Only defined for text posts
	Body string `json:"body"`

	// Only defined for photo posts
	Photos []photo `json:"photos"`

	// Only defined for video posts
	VideoURL string `json:"video_url"`

	// Only defined for answer posts
	Answer string `json:"answer"`

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

type trailEntry struct {
	Blog struct {
		Name string `json:"name"`
	} `json:"blog"`
	ContentRaw    string `json:"content_raw"`
	Content       string `json:"content"`
	IsRootItem    bool   `json:"is_root_item"`
	IsCurrentItem bool   `json:"is_current_item"`
}

type reblog struct {
	Comment string `json:"comment"`
}

type content struct {
}
