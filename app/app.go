package app

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/lhecker/tumblr-scraper/cookiejar"
)

func New() *cli.App {
	return &cli.App{
		Name: "tumblr-scraper",
		Commands: []*cli.Command{
			newUpdateCommand(),
		},
	}
}

func terminationSignalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		defer signal.Stop(ch)

		<-ch
	}()

	return ctx
}

func newHTTPClient(jar *cookiejar.Jar) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 60 * time.Second,
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
