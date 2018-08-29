package config

import (
	"log"
	"strings"
	"time"

	"github.com/burntsushi/toml"
)

// Config is a struct that contains all the configuration options
// and possibilities for the downloader to run.
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
	Name   string `toml:"Name"`
	Target string `toml:"target"`

	// Optional
	IgnoreReblogs bool      `toml:"ignore_reblogs"`
	Rescrape      bool      `toml:"rescrape"`
	Before        time.Time `toml:"before"`
}

func NewConfig(path string) *Config {
	cfg := &Config{}

	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		log.Panic(err)
	}

	configEnsureTrue(len(cfg.APIKey) != 0, "api_key")
	configEnsureTrue(len(cfg.Blogs) != 0, "blogs")
	for idx, blog := range cfg.Blogs {
		configEnsureTrue(blog != nil, "blogs[%d]", idx)
		configEnsureTrue(len(blog.Name) != 0, "blogs[%d].Name", idx)
		configEnsureTrue(len(blog.Target) != 0, "blogs[%d].target", idx)
	}

	if len(cfg.Username) != 0 {
		configEnsureTrue(len(cfg.Password) != 0, "password")
	}

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}

	for _, blog := range cfg.Blogs {
		if !strings.ContainsRune(blog.Name, '.') {
			blog.Name += ".tumblr.com"
		}
	}

	return cfg
}

func configEnsureTrue(value bool, format string, v ...interface{}) {
	if !value {
		log.Panicf("missing config value: "+format, v...)
	}
}
