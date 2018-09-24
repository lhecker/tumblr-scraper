package app

import (
	"log"

	"gopkg.in/urfave/cli.v2"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
	"github.com/lhecker/tumblr-scraper/scraper"
)

func newUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:   "update",
		Action: handleUpdate,
	}
}

func handleUpdate(c *cli.Context) error {
	ctx, cancel := terminationSignalContext()
	defer cancel()

	configPath := "tumblr.toml"
	cfg, err := config.LoadConfigOrDefault(configPath)
	if err != nil {
		return err
	}

	db, err := database.NewDatabase()
	if err != nil {
		return err
	}

	httpClient := newHTTPClient()

	defer func() {
		err := account.Logout()
		if err != nil {
			log.Printf("failed to logout: %v", err)
		}
	}()

	if len(cfg.Username) != 0 {
		account.Setup(httpClient, cfg)
	}

	s := scraper.NewScraper(httpClient, cfg, db)

	for _, blog := range cfg.Blogs {
		highestPostID, err := s.Scrape(ctx, blog)
		if err != nil {
			if !isContextCanceledError(err) {
				log.Println(err)
			}
			return err
		}

		err = db.SetHighestID(blog.Name, highestPostID)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	cfg.Save(configPath)
	return nil
}
