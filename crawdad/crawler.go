package crawdad

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
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

	"github.com/schollz/pluck/pluck"

	"golang.org/x/net/proxy"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-redis/redis"
	"github.com/goware/urlx"
	"github.com/jcelliott/lumber"
	"github.com/schollz/collectlinks"
)

// Settings is the configuration across all instances
type Settings struct {
	BaseURL              string
	PluckConfig          string
	KeywordsToExclude    []string
	KeywordsToInclude    []string
	AllowQueryParameters bool
	AllowHashParameters  bool
	DontFollowLinks      bool
}

// Crawler is the crawler instance
type Crawler struct {
	// Instance options
	RedisURL                 string
	RedisPort                string
	MaxNumberConnections     int
	MaxNumberWorkers         int
	MaximumNumberOfErrors    int
	TimeIntervalToPrintStats int
	Verbose                  bool
	UseProxy                 bool
	UserAgent                string

	// Public  options
	Settings Settings

	// Private instance parameters
	log                *lumber.ConsoleLogger
	programTime        time.Time
	numberOfURLSParsed int
	numTrash           int64
	numDone            int64
	numToDo            int64
	numDoing           int64
	isRunning          bool
	errors             int64
	client             *http.Client
	todo               *redis.Client
	doing              *redis.Client
	done               *redis.Client
	trash              *redis.Client
	wg                 sync.WaitGroup
}

// New creates a new crawler instance
func New() (*Crawler, error) {
	var err error
	err = nil
	c := new(Crawler)
	c.MaxNumberConnections = 20
	c.MaxNumberWorkers = 8
	c.RedisURL = "localhost"
	c.RedisPort = "6379"
	c.TimeIntervalToPrintStats = 1
	c.MaximumNumberOfErrors = 10
	c.errors = 0
	return c, err
}

// Init initializes the connection pool and the Redis client
func (c *Crawler) Init(config ...Settings) (err error) {
	// Generate the logging
	if c.Verbose {
		c.log = lumber.NewConsoleLogger(lumber.TRACE)
	} else {
		c.log = lumber.NewConsoleLogger(lumber.WARN)
	}

	// connect to Redis for the settings
	remoteSettings := redis.NewClient(&redis.Options{
		Addr:     c.RedisURL + ":" + c.RedisPort,
		Password: "",
		DB:       4,
	})
	_, err = remoteSettings.Ping().Result()
	if err != nil {
		return errors.New(fmt.Sprintf("Redis not available at %s:%s, did you run it? The easiest way is\n\n\tdocker run -d -v `pwd`:/data -p 6379:6379 redis\n\n", c.RedisURL, c.RedisPort))
	}
	if len(config) > 0 {
		// save the supplied configuration to Redis
		bSettings, err := json.Marshal(config[0])
		_, err = remoteSettings.Set("settings", string(bSettings), 0).Result()
		if err != nil {
			return err
		}
		c.log.Info("saved settings: %v", config[0])
	}
	// load the configuration from Redis
	var val string
	val, err = remoteSettings.Get("settings").Result()
	if err != nil {
		return errors.New(fmt.Sprintf("You need to set the base settings. Use\n\n\tcrawdad -s %s -p %s -set -url http://www.URL.com\n\n", c.RedisURL, c.RedisPort))
	}
	err = json.Unmarshal([]byte(val), &c.Settings)
	c.log.Info("loaded settings: %v", c.Settings)

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

	c.AddSeeds([]string{c.Settings.BaseURL})
	return
}

func (c *Crawler) Redo() (err error) {
	var keys []string
	keys, err = c.doing.Keys("*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		c.log.Trace("Moving %s back to todo list", key)
		_, err = c.doing.Del(key).Result()
		if err != nil {
			c.log.Error(err.Error())
		}
		_, err = c.todo.Set(key, "", 0).Result()
		if err != nil {
			c.log.Error(err.Error())
		}
	}

	keys, err = c.trash.Keys("*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		c.log.Trace("Moving %s back to todo list", key)
		_, err = c.trash.Del(key).Result()
		if err != nil {
			c.log.Error(err.Error())
		}
		_, err = c.todo.Set(key, "", 0).Result()
		if err != nil {
			c.log.Error(err.Error())
		}
	}

	return
}

func (c *Crawler) DumpMap() (m map[string]string, err error) {
	var keySize int64
	var keys []string
	keySize, _ = c.done.DbSize().Result()
	keys = make([]string, keySize+10000)
	i := 0
	iter := c.done.Scan(0, "", 0).Iterator()
	for iter.Next() {
		keys[i] = iter.Val()
		i++
	}
	keys = keys[:i]
	if err = iter.Err(); err != nil {
		c.log.Error("Problem getting done")
		return
	}
	m = make(map[string]string)
	for _, key := range keys {
		var val string
		val, err = c.done.Get(key).Result()
		if err != nil {
			return
		}
		m[key] = val
	}
	return
}

func (c *Crawler) Dump() (allKeys []string, err error) {
	allKeys = make([]string, 0)
	var keySize int64
	var keys []string

	keySize, _ = c.todo.DbSize().Result()
	keys = make([]string, keySize)
	i := 0
	iter := c.todo.Scan(0, "", 0).Iterator()
	for iter.Next() {
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting todo")
		return nil, err
	}
	allKeys = append(allKeys, keys...)

	keySize, _ = c.doing.DbSize().Result()
	keys = make([]string, keySize)
	i = 0
	iter = c.doing.Scan(0, "", 0).Iterator()
	for iter.Next() {
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting doing")
		return nil, err
	}
	allKeys = append(allKeys, keys...)

	keySize, _ = c.done.DbSize().Result()
	keys = make([]string, keySize)
	i = 0
	iter = c.done.Scan(0, "", 0).Iterator()
	for iter.Next() {
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting done")
		return nil, err
	}
	allKeys = append(allKeys, keys...)

	keySize, _ = c.trash.DbSize().Result()
	keys = make([]string, keySize)
	i = 0
	iter = c.trash.Scan(0, "", 0).Iterator()
	for iter.Next() {
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting trash")
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

func (c *Crawler) scrapeLinks(url string) (linkCandidates []string, pluckedData string, err error) {
	c.log.Trace("Scraping %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.log.Error("Problem making request for %s: %s", url, err.Error())
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

	if resp.StatusCode == 404 {
		c.doing.Del(url).Result()
		c.todo.Del(url).Result()
		c.trash.Set(url, "", 0).Result()
		return
	} else if resp.StatusCode != 200 {
		c.errors++
		if c.errors > int64(c.MaximumNumberOfErrors) {
			fmt.Println("Too many errors, exiting!")
			os.Exit(1)
		}
	}
	// reset errors as long as the code is good
	c.errors = 0

	// copy resp.Body
	var bodyBytes []byte
	bodyBytes, _ = ioutil.ReadAll(resp.Body)
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	// do plucking
	if c.Settings.PluckConfig != "" {
		plucker, _ := pluck.New()
		err = plucker.LoadFromString(c.Settings.PluckConfig)
		if err != nil {
			return
		}
		err = plucker.Pluck(bufio.NewReader(bytes.NewReader(bodyBytes)))
		if err != nil {
			return
		}
		pluckedData = plucker.ResultJSON()
	}

	if c.Settings.DontFollowLinks {
		return
	}

	// collect links
	links := collectlinks.All(resp.Body)

	// find good links
	linkCandidates = make([]string, len(links))
	linkCandidatesI := 0
	for _, link := range links {
		c.log.Trace(link)
		// disallow query parameters, if not flagged
		if strings.Contains(link, "?") && !c.Settings.AllowQueryParameters {
			link = strings.Split(link, "?")[0]
		}

		// disallow hash parameters, if not flagged
		if strings.Contains(link, "#") && !c.Settings.AllowHashParameters {
			link = strings.Split(link, "#")[0]
		}

		// add Base URL if it doesn't have
		if !strings.Contains(link, "http") && len(link) > 2 {
			if c.Settings.BaseURL[len(c.Settings.BaseURL)-1] != '/' && link[0] != '/' {
				link = "/" + link
			}
			link = c.Settings.BaseURL + link
		}

		// skip links that have a different Base URL
		if !strings.Contains(link, c.Settings.BaseURL) {
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
		for _, keyword := range c.Settings.KeywordsToExclude {
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
		for _, keyword := range c.Settings.KeywordsToInclude {
			if strings.Contains(normalizedLink, keyword) {
				foundIncludedKeyword = true
				break
			}
		}
		if !foundIncludedKeyword && len(c.Settings.KeywordsToInclude) > 0 {
			continue
		}

		// If it passed all the tests, add to link candidates
		linkCandidates[linkCandidatesI] = normalizedLink
		linkCandidatesI++
	}
	// trim candidate list
	linkCandidates = linkCandidates[0:linkCandidatesI]

	return
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
		urls, pluckedData, err := c.scrapeLinks(randomURL)
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
		_, err = c.done.Set(randomURL, pluckedData, 0).Result()
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

func (c *Crawler) AddSeeds(seeds []string) (err error) {
	// add beginning link
	for _, seed := range seeds {
		err = c.addLinkToDo(seed, true)
		if err != nil {
			return
		}
	}
	c.log.Info("Added %d seed links", len(seeds))
	return
}

// Crawl initiates the pool of connections and begins
// scraping URLs according to the todo list
func (c *Crawler) Crawl() (err error) {
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
		c.Settings.BaseURL,
		humanize.Comma(int64(c.numberOfURLSParsed)),
		URLSPerSecond,
		humanize.Comma(int64(c.numToDo)),
		humanize.Comma(int64(c.numDone)),
		humanize.Comma(int64(c.numDoing)),
		humanize.Comma(int64(c.numTrash)),
		humanize.Comma(int64(c.errors)))
}
