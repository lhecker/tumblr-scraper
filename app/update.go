package app

import (
	"log"

	"github.com/urfave/cli/v2"

	"github.com/lhecker/tumblr-scraper/account"
	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/cookiejar"
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
	ctx := terminationSignalContext()

	configPath := "tumblr.toml"
	cfg, err := config.LoadConfigOrDefault(configPath)
	if err != nil {
		return err
	}

	db, err := database.NewDatabase()
	if err != nil {
		return err
	}

	cookieSnapshot, err := db.GetCookies()
	if err != nil {
		log.Printf("failed to get cookie snapshot: %v", err)
	}

	jar := cookiejar.New(cookieSnapshot)
	if err != nil {
		return err
	}
	defer func() {
		snapshot := jar.Snapshot()
		err := db.SaveCookies(snapshot)
		if err != nil {
			log.Printf("failed to save cookies: %v", err)
		}
	}()

	httpClient := newHTTPClient(jar)

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
