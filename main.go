package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	crawler "github.com/schollz/goredis-crawler/lib"
	"github.com/urfave/cli"
)

var version string

func main() {
	app := cli.NewApp()
	app.Name = "goredis-crawler"
	app.Usage = "crawl a site for links"
	app.Version = version
	app.Compiled = time.Now()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "url, u",
			Value: "",
			Usage: "base URL to crawl",
		},
		cli.StringFlag{
			Name:  "seed",
			Value: "",
			Usage: "URL to seed with",
		},
		cli.StringFlag{
			Name:  "server, s",
			Value: "localhost",
			Usage: "address for Redis server",
		},
		cli.StringFlag{
			Name:  "port, p",
			Value: "6379",
			Usage: "port for Redis server",
		},
		cli.StringFlag{
			Name:  "exclude, e",
			Value: "",
			Usage: "comma-delimted phrases that must NOT be in URL",
		},
		cli.StringFlag{
			Name:  "include, i",
			Value: "",
			Usage: "comma-delimted phrases that must be in URL",
		},
		cli.IntFlag{
			Name:  "stats",
			Value: 1,
			Usage: "Print stats every `X` seconds",
		},
		cli.IntFlag{
			Name:  "connections, c",
			Value: 25,
			Usage: "number of connections to use",
		},
		cli.IntFlag{
			Name:  "workers, w",
			Value: 8,
			Usage: "number of connections to use",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "turn on logging",
		},
		cli.BoolFlag{
			Name:  "proxy",
			Usage: "use tor proxy",
		},
		cli.StringFlag{
			Name:  "dump",
			Value: "",
			Usage: "dump the records to `file`",
		},
		cli.StringFlag{
			Name:  "useragent",
			Value: "",
			Usage: "set the specified `useragent`",
		},
		cli.BoolFlag{
			Name:  "redo",
			Usage: "move items from 'doing' to 'todo'",
		},
		cli.BoolFlag{
			Name:  "query",
			Usage: "allow query parameters in URL",
		},
		cli.BoolFlag{
			Name:  "hash",
			Usage: "allow hashes in URL",
		},
	}

	app.Action = func(c *cli.Context) error {
		// Setup crawler to crawl
		url := c.GlobalString("url")
		craw, err := crawler.New(url)
		if err != nil {
			return err
		}
		craw.MaxNumberConnections = c.GlobalInt("connections")
		craw.MaxNumberWorkers = c.GlobalInt("workers")
		craw.Verbose = c.GlobalBool("verbose")
		craw.TimeIntervalToPrintStats = c.GlobalInt("stats")
		craw.UserAgent = c.GlobalString("useragent")
		craw.AllowQueryParameters = c.GlobalBool("query")
		craw.AllowHashParameters = c.GlobalBool("hash")
		craw.UseProxy = c.GlobalBool("proxy")
		craw.RedisPort = c.GlobalString("port")
		craw.RedisURL = c.GlobalString("server")
		if len(c.GlobalString("seed")) > 0 {
			craw.SeedURL = c.GlobalString("seed")
		}
		if len(c.GlobalString("include")) > 0 {
			craw.KeywordsToInclude = strings.Split(strings.ToLower(c.GlobalString("include")), ",")
		}
		if len(c.GlobalString("exclude")) > 0 {
			craw.KeywordsToExclude = strings.Split(strings.ToLower(c.GlobalString("exclude")), ",")
		}
		err = craw.Init()
		if err != nil {
			return err
		}
		if c.GlobalString("dump") != "" {
			var allKeys []string
			allKeys, err = craw.Dump()
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(c.GlobalString("dump"), []byte(strings.Join(allKeys, "\n")), 0644)
			fmt.Printf("Wrote %d keys to '%s'\n", len(allKeys), c.GlobalString("dump"))
		} else if c.GlobalBool("redo") {
			err = craw.Redo()
		} else {
			if url == "" {
				fmt.Println("You should specify a URL to crawl, --url URL")
				return nil
			}
			fmt.Printf("Starting crawl on %s\n\n", url)
			err = craw.Crawl()
		}
		return err
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Print(err)
	}
}
