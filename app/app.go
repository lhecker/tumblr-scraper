package app

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/publicsuffix"
	"gopkg.in/urfave/cli.v2"
)

func New() *cli.App {
	return &cli.App{
		Name: "tumblr-scraper",
		Commands: []*cli.Command{
			newUpdateCommand(),
		},
	}
}

func terminationSignalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		defer signal.Stop(ch)

		select {
		case <-ch:
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func newHTTPClient() *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		panic(err)
	}

	return &http.Client{
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
		Jar:     jar,
	}
}

func isContextCanceledError(err error) bool {
	if e, ok := err.(*url.Error); ok {
		err = e.Err
	}
	return err == context.Canceled
}
