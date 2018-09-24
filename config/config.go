package config

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	backupExtension = ".bak"
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
	AllowReblogsFrom *[]string `toml:"allow_reblogs_from"`
	Rescrape         bool      `toml:"rescrape"`
	Before           time.Time `toml:"before"`
}

func LoadConfigOrDefault(path string) (*Config, error) {
	cfg, err := loadConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		cfg, err = loadConfig(path + backupExtension)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}

			log.Print("config file not found - using default values")
		} else {
			log.Print("recovering backup config file")
		}
	}

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 24
	}

	for _, blog := range cfg.Blogs {
		blog.Name = TumblrNameToUUID(blog.Name)

		if blog.AllowReblogsFrom != nil {
			for idx, from := range *blog.AllowReblogsFrom {
				(*blog.AllowReblogsFrom)[idx] = TumblrNameToUUID(from)
			}
		}
	}

	return cfg, nil
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{}

	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (s *Config) Save(path string) {
	var err error
	defer func() {
		if err != nil {
			log.Printf("failed to save config: %v", err)
		}
	}()

	data := &bytes.Buffer{}
	err = toml.NewEncoder(data).Encode(s)
	if err != nil {
		return
	}

	info, err := os.Lstat(path)
	if err != nil {
		return
	}

	backupPath := path + backupExtension
	err = os.Rename(path, backupPath)
	if err != nil {
		return
	}

	err = ioutil.WriteFile(path, data.Bytes(), info.Mode())
	if err != nil {
		return
	}

	err = os.Remove(backupPath)
	return
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
