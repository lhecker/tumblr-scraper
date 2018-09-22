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

type BlogConfig struct {
	// Required
	Name   string `toml:"name"`
	Target string `toml:"target"`

	// Optional
	IgnoreReblogs bool      `toml:"ignore_reblogs"`
	Rescrape      bool      `toml:"rescrape"`
	Before        time.Time `toml:"before"`
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
