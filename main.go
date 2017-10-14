package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/schollz/crawdad/crawdad"
	"github.com/urfave/cli"
)

var version string

func main() {
	app := cli.NewApp()
	app.Name = "crawdad"
	app.Usage = "crawl a site for links"
	app.Version = version
	app.Compiled = time.Now()
	app.Flags = []cli.Flag{
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
			Name:  "url, u",
			Value: "",
			Usage: "set base URL to crawl",
		},
		cli.StringFlag{
			Name:  "exclude, e",
			Value: "",
			Usage: "set comma-delimted phrases that must NOT be in URL",
		},
		cli.StringFlag{
			Name:  "include, i",
			Value: "",
			Usage: "set comma-delimted phrases that must be in URL",
		},
		cli.StringFlag{
			Name:  "seed",
			Value: "",
			Usage: "`file` with URLs to add to queue",
		},
		cli.StringFlag{
			Name:  "pluck",
			Value: "",
			Usage: "set config file for a plucker (see github.com/schollz/pluck)",
		},
		cli.BoolFlag{
			Name:  "require-pluck",
			Usage: "requires that some plucked content is found",
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
			Name:  "info",
			Usage: "turn on basic logging",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "turn on LOTS of logging",
		},
		cli.BoolFlag{
			Name:  "proxy",
			Usage: "use tor proxy",
		},
		cli.BoolFlag{
			Name:  "set",
			Usage: "set options across crawdads",
		},
		cli.BoolFlag{
			Name:  "flush",
			Usage: "flush entire database",
		},
		cli.StringFlag{
			Name:  "dump",
			Value: "",
			Usage: "dump all the keys to `file`",
		},
		cli.StringFlag{
			Name:  "done",
			Value: "",
			Usage: "dump the map of the done things `file`",
		},
		cli.StringFlag{
			Name:  "useragent",
			Value: "",
			Usage: "set the specified `useragent`",
		},
		cli.StringFlag{
			Name:  "cookie",
			Value: "",
			Usage: "set the specified `cookie` header",
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
		cli.BoolFlag{
			Name:  "no-follow",
			Usage: "do not follow links (useful with -seed)",
		},
		cli.IntFlag{
			Name:  "errors",
			Value: 10,
			Usage: "maximum number of errors before exiting",
		},
	}

	app.Action = func(c *cli.Context) error {
		// Show start-up stuff
		fmt.Printf(`
                      _______
             \\ //   /   -^--\ |  
             ||||   / /\_____/ /  
  {\         ______{ }        /   
  {_}{\{\{\{|         \=@____/    
 <{_{-{-{-{-| ====---- >>>        
  { }{/{/{/{|______  _/=@_____    
  {/               { }        \   
             ||||  \ \______   \  
             // \\   \    _^_\ |  
                      \______/   
                       
	crawdad version ` + app.Version + "\n\n")
		// Setup crawler to crawl
		craw, err := crawdad.New()
		if err != nil {
			return err
		}
		// set instance options
		craw.MaxNumberConnections = c.GlobalInt("connections")
		craw.MaxNumberWorkers = c.GlobalInt("workers")
		craw.Info = c.GlobalBool("info")
		craw.Debug = c.GlobalBool("debug")
		craw.TimeIntervalToPrintStats = c.GlobalInt("stats")
		craw.UserAgent = c.GlobalString("useragent")
		craw.Cookie = c.GlobalString("cookie")

		// set public options
		var options crawdad.Settings
		if c.GlobalBool("set") {
			options.BaseURL = c.GlobalString("url")
			options.AllowQueryParameters = c.GlobalBool("query")
			options.AllowHashParameters = c.GlobalBool("hash")
			options.DontFollowLinks = c.GlobalBool("no-follow")
			options.RequirePluck = c.GlobalBool("require-pluck")
			if len(c.GlobalString("include")) > 0 {
				options.KeywordsToInclude = strings.Split(strings.ToLower(c.GlobalString("include")), ",")
			}
			if len(c.GlobalString("exclude")) > 0 {
				options.KeywordsToExclude = strings.Split(strings.ToLower(c.GlobalString("exclude")), ",")
			}
			if len(c.GlobalString("pluck")) > 0 {
				bFile, errFile := ioutil.ReadFile(c.GlobalString("pluck"))
				if errFile != nil {
					return errFile
				}
				options.PluckConfig = string(bFile)
			}
		}
		craw.UseProxy = c.GlobalBool("proxy")
		craw.RedisPort = c.GlobalString("port")
		craw.RedisURL = c.GlobalString("server")
		craw.MaximumNumberOfErrors = c.GlobalInt("errors")
		craw.EraseDB = c.GlobalBool("flush")
		if c.GlobalBool("set") {
			err = craw.Init(options)
		} else {
			err = craw.Init()
		}
		if err != nil {
			return err
		}
		craw.Logging()

		if c.GlobalString("seed") != "" {
			seedData, err := ioutil.ReadFile(c.GlobalString("seed"))
			if err != nil {
				return err
			}
			seeds := make([]string, len(bytes.Split(seedData, []byte("\n"))))
			for i, seed := range strings.Split(string(seedData), "\n") {
				seeds[i] = strings.TrimSpace(seed)
			}
			err = craw.AddSeeds(seeds)
			if err != nil {
				return err
			}
		}
		if c.GlobalString("dump") != "" {
			var allKeys []string
			allKeys, err = craw.Dump()
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(c.GlobalString("dump"), []byte(strings.Join(allKeys, "\n")), 0644)
			fmt.Printf("Wrote %d keys to '%s'\n", len(allKeys), c.GlobalString("dump"))
		} else if c.GlobalString("done") != "" {
			m, err2 := craw.DumpMap()
			if err2 != nil {
				return err2
			}

			b, err2 := json.MarshalIndent(m, "", " ")
			if err2 != nil {
				return err2
			}
			err = ioutil.WriteFile(c.GlobalString("done"), b, 0644)
			fmt.Printf("Wrote %d keys to '%s'\n", len(m), c.GlobalString("done"))
		} else if c.GlobalBool("redo") {
			err = craw.Redo()
		} else {
			err = craw.Crawl()
		}
		return err
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Print(err)
	}
}
