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
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`

	// Always defined for indash_blog posts
	PostHTML string `json:"post_html"`

	// Only defined for text posts
	Body string `json:"body"`

	// Only defined for photo posts
	Photos []photo `json:"photos"`

	// Only defined for video posts
	VideoURL string `json:"video_url"`

	// Only defined for answer posts
	Answer string `json:"answer"`

	// Only defined for reblogs
	RebloggedFromID json.Number `json:"reblogged_from_id"`
}

func (s *post) timestamp() time.Time {
	return time.Unix(s.Timestamp, 0)
}

type photo struct {
	OriginalSize photoVariant   `json:"original_size"`
	AltSizes     []photoVariant `json:"alt_sizes"`
}

type photoVariant struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
