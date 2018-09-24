package main

import (
	"fmt"
	"os"

	"gopkg.in/urfave/cli.v2"

	"github.com/lhecker/tumblr-scraper/app"
)

func main() {
	err := app.New().Run(os.Args)
	if err != nil {
		fmt.Fprintln(cli.ErrWriter, err)
		cli.OsExiter(1)
	}
}
