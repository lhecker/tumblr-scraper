package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/scraper"
)

var (
	updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Scrape multiple blogs at once",
		Run:   updateRun,
	}
)

func init() {
	rootCmd.AddCommand(updateCmd)
}

func updateRun(cmd *cobra.Command, args []string) {
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

	if len(singletons.Config.Username) != 0 {
		account.Setup(singletons.HTTPClient, singletons.Config)
	}

	s := scraper.NewScraper(singletons.HTTPClient, singletons.Config, singletons.Database)

	for _, blog := range singletons.Config.Blogs {
		highestPostID, err := s.Scrape(ctx, blog)
		if err != nil {
			if !isContextCanceledError(err) {
				log.Println(err)
			}
			return
		}

		err = singletons.Database.SetHighestID(blog.Name, highestPostID)
		if err != nil {
			log.Println(err)
			return
		}
	}
}
