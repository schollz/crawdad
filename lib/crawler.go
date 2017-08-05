package crawler

import (
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-redis/redis"
	"github.com/goware/urlx"
	"github.com/jcelliott/lumber"
	"github.com/schollz/collectlinks"
)

// Crawler is the crawler instance
type Crawler struct {
	client                   *http.Client
	todo                     *redis.Client
	doing                    *redis.Client
	done                     *redis.Client
	trash                    *redis.Client
	wg                       sync.WaitGroup
	MaxNumberConnections     int
	MaxNumberWorkers         int
	BaseURL                  string
	RedisURL                 string
	RedisPort                string
	KeywordsToExclude        []string
	KeywordsToInclude        []string
	Verbose                  bool
	UseProxy                 bool
	UserAgent                string
	log                      *lumber.ConsoleLogger
	programTime              time.Time
	numberOfURLSParsed       int
	TimeIntervalToPrintStats int
	numTrash                 int64
	numDone                  int64
	numToDo                  int64
	numDoing                 int64
	isRunning                bool
	errors                   int64
}

// New creates a new crawler instance
func New(baseurl string) (*Crawler, error) {
	var err error
	err = nil
	c := new(Crawler)
	c.BaseURL = baseurl
	c.MaxNumberConnections = 20
	c.MaxNumberWorkers = 8
	c.UserAgent = ""
	c.RedisURL = "localhost"
	c.RedisPort = "6379"
	c.TimeIntervalToPrintStats = 1
	c.errors = 0
	return c, err
}

// Init initializes the connection pool and the Redis client
func (c *Crawler) Init() error {
	// Generate the logging
	if c.Verbose {
		c.log = lumber.NewConsoleLogger(lumber.TRACE)
	} else {
		c.log = lumber.NewConsoleLogger(lumber.WARN)
	}

	// Generate the connection pool
	var tr *http.Transport
	if c.UseProxy {
		tbProxyURL, err := url.Parse("socks5://127.0.0.1:9050")
		if err != nil {
			c.log.Fatal("Failed to parse proxy URL: %v\n", err)
			return err
		}
		tbDialer, err := proxy.FromURL(tbProxyURL, proxy.Direct)
		if err != nil {
			c.log.Fatal("Failed to obtain proxy dialer: %v\n", err)
			return err
		}
		tr = &http.Transport{
			MaxIdleConns:       c.MaxNumberConnections,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: true,
			Dial:               tbDialer.Dial,
		}
	} else {
		tr = &http.Transport{
			MaxIdleConns:       c.MaxNumberConnections,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: true,
		}
	}
	c.client = &http.Client{
		Transport: tr,
		Timeout:   time.Duration(15 * time.Second),
	}

	// Setup Redis client
	c.todo = redis.NewClient(&redis.Options{
		Addr:     c.RedisURL + ":" + c.RedisPort,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	c.doing = redis.NewClient(&redis.Options{
		Addr:     c.RedisURL + ":" + c.RedisPort,
		Password: "", // no password set
		DB:       1,  // use default DB
	})
	c.done = redis.NewClient(&redis.Options{
		Addr:     c.RedisURL + ":" + c.RedisPort,
		Password: "", // no password set
		DB:       2,  // use default DB
	})
	c.trash = redis.NewClient(&redis.Options{
		Addr:     c.RedisURL + ":" + c.RedisPort,
		Password: "", // no password set
		DB:       3,  // use default DB
	})
	_, err := c.todo.Ping().Result()
	if err != nil {
		fmt.Printf("Redis not available at %s:%s, did you run it?\nThe easiest way is\ndocker run -p 6379:6379 redis\n\n", c.RedisURL, c.RedisPort)
	}
	return err
}

func (c *Crawler) Dump() (allKeys []string, err error) {
	var keys []string
	keys, err = c.todo.Keys("*").Result()
	if err != nil {
		return nil, err
	}
	allKeys = append(allKeys, keys...)
	keys, err = c.doing.Keys("*").Result()
	if err != nil {
		return nil, err
	}
	allKeys = append(allKeys, keys...)
	keys, err = c.done.Keys("*").Result()
	if err != nil {
		return nil, err
	}
	allKeys = append(allKeys, keys...)
	keys, err = c.trash.Keys("*").Result()
	if err != nil {
		return nil, err
	}
	allKeys = append(allKeys, keys...)
	return
}

func (c *Crawler) getIP() (ip string, err error) {
	req, err := http.NewRequest("GET", "http://icanhazip.com", nil)
	if err != nil {
		c.log.Error("Problem making request")
		return
	}
	if c.UserAgent != "" {
		c.log.Trace("Setting useragent string to '%s'", c.UserAgent)
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	ipB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip = string(ipB)
	return
}

func (c *Crawler) addLinkToDo(link string, force bool) (err error) {
	if !force {
		// add only if it isn't already in one of the databases
		_, err = c.todo.Get(link).Result()
		if err != redis.Nil {
			return
		}
		_, err = c.doing.Get(link).Result()
		if err != redis.Nil {
			return
		}
		_, err = c.done.Get(link).Result()
		if err != redis.Nil {
			return
		}
		_, err = c.trash.Get(link).Result()
		if err != redis.Nil {
			return
		}
	}

	// add it to the todo list
	err = c.todo.Set(link, "", 0).Err()
	return
}

func (c *Crawler) scrapeLinks(url string) ([]string, error) {
	c.log.Trace("Scraping %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.log.Error("Problem making request for %s: %s", url, err.Error())
		return nil, err
	}
	if c.UserAgent != "" {
		c.log.Trace("Setting useragent string to '%s'", c.UserAgent)
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		c.doing.Del(url).Result()
		c.todo.Del(url).Result()
		c.trash.Set(url, "", 0).Result()
		return []string{}, err
	} else if resp.StatusCode != 200 {
		c.errors++
		if c.errors > 10 {
			fmt.Println("Too many errors, exiting!")
			os.Exit(1)
		}
	}

	// collect links
	links := collectlinks.All(resp.Body)

	// find good links
	linkCandidates := make([]string, len(links))
	linkCandidatesI := 0
	for _, link := range links {
		// disallow query parameters
		if strings.Contains(link, "?") {
			link = strings.Split(link, "?")[0]
		}

		// add Base URL if it doesn't have
		if !strings.Contains(link, "http") {
			link = c.BaseURL + link
		}

		// skip links that have a different Base URL
		if !strings.Contains(link, c.BaseURL) {
			// c.log.Trace("Skipping %s because it has a different base URL", link)
			continue
		}

		// normalize the link
		parsedLink, _ := urlx.Parse(link)
		normalizedLink, _ := urlx.Normalize(parsedLink)
		if len(normalizedLink) == 0 {
			continue
		}

		// Exclude keywords, skip if any are found
		foundExcludedKeyword := false
		for _, keyword := range c.KeywordsToExclude {
			if strings.Contains(normalizedLink, keyword) {
				foundExcludedKeyword = true
				// c.log.Trace("Skipping %s because contains %s", link, keyword)
				break
			}
		}
		if foundExcludedKeyword {
			continue
		}

		// Include keywords, skip if any are NOT found
		foundIncludedKeyword := false
		for _, keyword := range c.KeywordsToInclude {
			if strings.Contains(normalizedLink, keyword) {
				foundIncludedKeyword = true
				break
			}
		}
		if !foundIncludedKeyword && len(c.KeywordsToInclude) > 0 {
			continue
		}

		// If it passed all the tests, add to link candidates
		linkCandidates[linkCandidatesI] = normalizedLink
		linkCandidatesI++
	}
	// trim candidate list
	linkCandidates = linkCandidates[0:linkCandidatesI]

	return linkCandidates, err
}

func (c *Crawler) crawl(id int, jobs <-chan int, results chan<- bool) {
	for j := range jobs {
		// time the link getting process
		t := time.Now()

		// check if there are any links to do
		dbsize, err := c.todo.DbSize().Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}

		// break if there are no links to do
		if dbsize == 0 {
			c.log.Trace("Exiting, no links")
			results <- false
			continue
		}

		// pop a URL
		randomURL, err := c.todo.RandomKey().Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}

		// place in 'doing'
		_, err = c.todo.Del(randomURL).Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}
		_, err = c.doing.Set(randomURL, "", 0).Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}

		c.log.Trace("Got work in %s", time.Since(t).String())
		urls, err := c.scrapeLinks(randomURL)
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}

		t = time.Now()
		// move url to 'done'
		_, err = c.doing.Del(randomURL).Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}
		_, err = c.done.Set(randomURL, "", 0).Result()
		if err != nil {
			c.log.Error(err.Error())
			results <- false
			continue
		}

		// add new urls to 'todo'
		for _, url := range urls {
			c.addLinkToDo(url, false)
		}
		if len(urls) > 0 {
			c.log.Trace("%d-%d %d urls from %s", id, j, len(urls), randomURL)
		}
		c.numberOfURLSParsed++
		c.log.Trace("Returned results in %s", time.Since(t).String())
		results <- true
	}
}

// Crawl initiates the pool of connections and begins
// scraping URLs according to the todo list
func (c *Crawler) Crawl() (err error) {
	// add beginning link
	c.addLinkToDo(c.BaseURL, true)

	c.programTime = time.Now()
	c.numberOfURLSParsed = 0
	it := 0
	go c.contantlyPrintStats()
	for {
		it++
		jobs := make(chan int, c.MaxNumberConnections)
		results := make(chan bool, c.MaxNumberConnections)

		// This starts up 3 workers, initially blocked
		// because there are no jobs yet.
		for w := 1; w <= c.MaxNumberWorkers; w++ {
			go c.crawl(w, jobs, results)
		}

		// Here we send 5 `jobs` and then `close` that
		// channel to indicate that's all the work we have.
		for j := 1; j <= c.MaxNumberConnections; j++ {
			jobs <- j
		}
		close(jobs)

		// Finally we collect all the results of the work.
		oneSuccess := false
		for a := 1; a <= c.MaxNumberConnections; a++ {
			success := <-results
			if success {
				oneSuccess = true
			}
		}

		if !oneSuccess {
			break
		}
	}
	c.isRunning = false
	return
}

func round(f float64) int {
	if math.Abs(f) < 0.5 {
		return 0
	}
	return int(f + math.Copysign(0.5, f))
}

func (c *Crawler) updateListCounts() (err error) {
	// Update stats
	c.numToDo, err = c.todo.DbSize().Result()
	if err != nil {
		return
	}
	c.numDoing, err = c.doing.DbSize().Result()
	if err != nil {
		return
	}
	c.numDone, err = c.done.DbSize().Result()
	if err != nil {
		return
	}
	c.numTrash, err = c.trash.DbSize().Result()
	if err != nil {
		return
	}
	return nil
}

func (c *Crawler) contantlyPrintStats() {
	c.isRunning = true
	for {
		time.Sleep(time.Duration(int32(c.TimeIntervalToPrintStats)) * time.Second)
		c.updateListCounts()
		c.printStats()
		if !c.isRunning {
			fmt.Println("Finished")
			return
		}
	}
}

func (c *Crawler) printStats() {
	URLSPerSecond := round(60.0 * float64(c.numberOfURLSParsed) / float64(time.Since(c.programTime).Seconds()))
	log.Printf("[%s]\t%s parsed (%d/min)\t%s todo\t%s done\t%s doing\t%s trashed\t%s errors\n",
		c.BaseURL,
		humanize.Comma(int64(c.numberOfURLSParsed)),
		URLSPerSecond,
		humanize.Comma(int64(c.numToDo)),
		humanize.Comma(int64(c.numDone)),
		humanize.Comma(int64(c.numDoing)),
		humanize.Comma(int64(c.numTrash)),
		humanize.Comma(int64(c.errors)))
}
