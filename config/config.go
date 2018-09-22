package config

import (
	"strings"
	"time"
)

type Config struct {
	// Required
	APIKey string        `toml:"api_key"`
	Blogs  []*BlogConfig `toml:"blogs"`

	// Optional
	Concurrency int    `toml:"concurrency"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
}

func (s *Config) Fixup() {
	if s.Concurrency <= 0 {
		s.Concurrency = 24
	}

	for _, blog := range s.Blogs {
		blog.Fixup()
	}
}

type BlogConfig struct {
	// Required
	Name   string `toml:"name"`
	Target string `toml:"target"`

	// Optional
	AllowReblogsFrom *[]string `toml:"allow_reblogs_from"`
	Rescrape         bool      `toml:"rescrape"`
	Before           time.Time `toml:"before"`
}

func (s *BlogConfig) Fixup() {
	s.Name = TumblrNameToUUID(s.Name)

	if s.AllowReblogsFrom != nil {
		for idx, from := range *s.AllowReblogsFrom {
			(*s.AllowReblogsFrom)[idx] = TumblrNameToUUID(from)
		}
	}
}

func TumblrUUIDToName(uuid string) string {
	return strings.TrimSuffix(uuid, ".tumblr.com")
}

func TumblrNameToUUID(name string) string {
	if strings.ContainsRune(name, '.') {
		return name
	}
	return name + ".tumblr.com"
}
