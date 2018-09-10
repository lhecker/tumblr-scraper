package cmd

import (
	"context"
	"net/http"
	"net/url"

	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
)

var (
	// This structure gets (de)initialized in the prerun and postrun hooks of the rootCmd.
	singletons = struct {
		Config     *config.Config
		Database   *database.Database
		HTTPClient *http.Client
	}{}
)

func isContextCanceledError(err error) bool {
	if e, ok := err.(*url.Error); ok {
		return e.Err == context.Canceled
	}
	return err == context.Canceled
}
