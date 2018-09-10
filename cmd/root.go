package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/publicsuffix"

	"github.com/lhecker/tumblr-scraper/config"
	"github.com/lhecker/tumblr-scraper/database"
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	configFile string

	rootCmd = &cobra.Command{
		Use:               "tumblr-scraper",
		Short:             "A crawler and scraper for Tumblr",
		PersistentPreRunE: rootPersistentPreRunE,
		PersistentPostRun: rootPersistentPostRun,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", `config file`)
}

func rootPersistentPreRunE(cmd *cobra.Command, args []string) error {
	for _, f := range []func() error{
		initHTTPClient,
		initConfig,
		initDatabase,
	} {
		err := f()
		if err != nil {
			return err
		}
	}

	return nil
}

func rootPersistentPostRun(cmd *cobra.Command, args []string) {
	for _, f := range []func(){
		deinitDatabase,
	} {
		f()
	}
}

func initHTTPClient() error {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return err
	}

	singletons.HTTPClient = &http.Client{
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
	return nil
}

func initConfig() (err error) {
	if len(configFile) != 0 {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("tumblr")
		viper.AddConfigPath(".")
	}

	viper.SetDefault("concurrency", 10)

	err = viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	singletons.Config = &config.Config{}
	defer func() {
		if err != nil {
			singletons.Config = nil
		}
	}()

	err = viper.Unmarshal(
		singletons.Config,
		func(config *mapstructure.DecoderConfig) {
			config.TagName = "toml"
			config.DecodeHook = mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeHookFunc(time.RFC3339),
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			)
		},
	)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	for _, blog := range singletons.Config.Blogs {
		if !strings.ContainsRune(blog.Name, '.') {
			blog.Name += ".tumblr.com"
		}
	}
	return
}

func initDatabase() (err error) {
	singletons.Database, err = database.NewDatabase()
	return
}

func deinitDatabase() {
	if singletons.Database == nil {
		return
	}

	err := singletons.Database.Close()
	if err != nil {
		log.Printf("failed to close database: %v", err)
	}
}
