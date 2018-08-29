package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
	"github.com/lhecker/tumblr-scraper/scraper"
)

func main() {
	defer func() {
		err := account.Logout()
		if err != nil {
			log.Printf("failed to logout: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		defer signal.Stop(ch)
		<-ch
	}()

	httpClient := newHTTPClient()
	cfg := config.NewConfig("tumblr.toml")

	if len(cfg.Username) != 0 {
		account.Setup(httpClient, cfg)
	}

	db := database.NewDatabase()
	defer db.Close()

	s := scraper.NewScraper(httpClient, cfg, db)

	for _, blog := range cfg.Blogs {
		highestPostID, err := s.Scrape(ctx, blog)
		if err != nil {
			if !isContextCanceledError(err) {
				log.Println(err)
			}
			return
		}

		db.SetHighestID(blog.Name, highestPostID)
	}
}

func newHTTPClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 60 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 60 * time.Second,
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	client.Jar = jar

	return client
}

func isContextCanceledError(err error) bool {
	if e, ok := err.(*url.Error); ok {
		return e.Err == context.Canceled
	}
	return err == context.Canceled
}
