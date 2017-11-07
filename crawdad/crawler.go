package crawdad

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/pluck/pluck"
	pb "gopkg.in/cheggaaa/pb.v1"

	"golang.org/x/net/proxy"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-redis/redis"
	"github.com/goware/urlx"
	"github.com/jcelliott/lumber"
	"github.com/pkg/errors"
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
	RequirePluck         bool
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
	Debug                    bool
	Info                     bool
	UseProxy                 bool
	UserAgent                string
	Cookie                   string
	EraseDB                  bool
	MaxQueueSize             int

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
	queue              *syncmap
	workersWorking     bool
}

type syncmap struct {
	Data map[string]struct{}
	sync.RWMutex
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
	c.MaximumNumberOfErrors = 20
	c.errors = 0
	c.MaxQueueSize = 500
	c.queue = new(syncmap)
	c.queue.Lock()
	c.queue.Data = make(map[string]struct{})
	c.queue.Unlock()
	return c, err
}

// Init initializes the connection pool and the Redis client
func (c *Crawler) Init(config ...Settings) (err error) {
	c.Logging()
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
			IdleConnTimeout:    15 * time.Second,
			DisableCompression: true,
			Dial:               tbDialer.Dial,
		}
	} else {
		tr = &http.Transport{
			MaxIdleConns:       c.MaxNumberConnections,
			IdleConnTimeout:    15 * time.Second,
			DisableCompression: true,
		}
	}
	c.client = &http.Client{
		Transport: tr,
		Timeout:   time.Duration(10 * time.Second),
	}

	// Setup Redis client
	c.todo = redis.NewClient(&redis.Options{
		Addr:        c.RedisURL + ":" + c.RedisPort,
		Password:    "", // no password set
		DB:          0,  // use default DB
		ReadTimeout: 30 * time.Second,
		MaxRetries:  10,
	})
	c.doing = redis.NewClient(&redis.Options{
		Addr:        c.RedisURL + ":" + c.RedisPort,
		Password:    "", // no password set
		DB:          1,  // use default DB
		ReadTimeout: 30 * time.Second,
		MaxRetries:  10,
	})
	c.done = redis.NewClient(&redis.Options{
		Addr:        c.RedisURL + ":" + c.RedisPort,
		Password:    "", // no password set
		DB:          2,  // use default DB
		ReadTimeout: 30 * time.Second,
		MaxRetries:  10,
	})
	c.trash = redis.NewClient(&redis.Options{
		Addr:        c.RedisURL + ":" + c.RedisPort,
		Password:    "", // no password set
		DB:          3,  // use default DB
		ReadTimeout: 30 * time.Second,
		MaxRetries:  10,
	})

	if c.EraseDB {
		c.log.Info("Flushed database")
		err = c.Flush()
		if err != nil {
			return err
		}
	}
	if len(c.Settings.BaseURL) > 0 {
		c.log.Info("Adding %s to URLs", c.Settings.BaseURL)
		err = c.addLinkToDo(c.Settings.BaseURL, true)
		if err != nil {
			return err
		}
	}
	return
}

func (c *Crawler) Logging() {
	// Generate the logging
	if c.Info {
		c.log = lumber.NewConsoleLogger(lumber.INFO)
	} else if c.Debug {
		c.log = lumber.NewConsoleLogger(lumber.TRACE)
	} else {
		c.log = lumber.NewConsoleLogger(lumber.WARN)
	}
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
	fmt.Println("Dumping...")
	totalSize := int64(0)
	var tempSize int64
	tempSize, _ = c.done.DbSize().Result()
	totalSize = tempSize * 2
	bar := pb.StartNew(int(totalSize))
	defer bar.Finish()

	var keySize int64
	var keys []string
	keySize, _ = c.done.DbSize().Result()
	keys = make([]string, keySize+10000)
	i := 0
	iter := c.done.Scan(0, "", 0).Iterator()
	for iter.Next() {
		bar.Increment()
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
		bar.Increment()
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
	fmt.Println("Dumping...")
	allKeys = make([]string, 0)
	var keySize int64
	var keys []string

	totalSize := int64(0)
	var tempSize int64
	tempSize, _ = c.todo.DbSize().Result()
	totalSize += tempSize
	tempSize, _ = c.done.DbSize().Result()
	totalSize += tempSize
	tempSize, _ = c.doing.DbSize().Result()
	totalSize += tempSize
	tempSize, _ = c.trash.DbSize().Result()
	totalSize += tempSize
	bar := pb.StartNew(int(totalSize))
	defer bar.Finish()

	keySize, _ = c.todo.DbSize().Result()
	keys = make([]string, keySize*2)
	i := 0
	iter := c.todo.Scan(0, "", 0).Iterator()
	for iter.Next() {
		bar.Increment()
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting todo")
		return nil, err
	}
	allKeys = append(allKeys, keys[:i]...)

	keySize, _ = c.doing.DbSize().Result()
	keys = make([]string, keySize*2)
	i = 0
	iter = c.doing.Scan(0, "", 0).Iterator()
	for iter.Next() {
		bar.Increment()
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting doing")
		return nil, err
	}
	allKeys = append(allKeys, keys[:i]...)

	keySize, _ = c.done.DbSize().Result()
	keys = make([]string, keySize*2)
	i = 0
	iter = c.done.Scan(0, "", 0).Iterator()
	for iter.Next() {
		bar.Increment()
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting done")
		return nil, err
	}
	allKeys = append(allKeys, keys[:i]...)

	keySize, _ = c.trash.DbSize().Result()
	keys = make([]string, keySize*2)
	i = 0
	iter = c.trash.Scan(0, "", 0).Iterator()
	for iter.Next() {
		bar.Increment()
		keys[i] = iter.Val()
		i++
	}
	if err := iter.Err(); err != nil {
		c.log.Error("Problem getting trash")
		return nil, err
	}
	allKeys = append(allKeys, keys[:i]...)
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

// Flush erases the database
func (c *Crawler) Flush() (err error) {
	_, err = c.todo.FlushAll().Result()
	if err != nil {
		return
	}
	_, err = c.done.FlushAll().Result()
	if err != nil {
		return
	}
	_, err = c.doing.FlushAll().Result()
	if err != nil {
		return
	}
	_, err = c.trash.FlushAll().Result()
	if err != nil {
		return
	}
	return
}

func (c *Crawler) scrapeLinks(url string) (linkCandidates []string, pluckedData string, err error) {
	c.log.Trace("Scraping %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		err = errors.Wrap(err, "could not make New Request for "+url)
		return
	}
	if c.UserAgent != "" {
		c.log.Trace("Setting useragent string to '%s'", c.UserAgent)
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Cookie != "" {
		c.log.Trace("Setting cookie")
		req.Header.Set("Cookie", c.Cookie)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "could not make do request for "+url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		c.doing.Del(url).Result()
		c.todo.Del(url).Result()
		c.trash.Set(url, "", 0).Result()
		if resp.StatusCode == 403 {
			c.errors++
			if c.errors > int64(c.MaximumNumberOfErrors) {
				err = errors.New(fmt.Sprintf("Got code %d for %s", resp.StatusCode, url))
				return
			}
		}
		return
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
		if c.Settings.RequirePluck && len(pluckedData) == 0 {
			err = errors.New("no data plucked from " + url)
			return
		}
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

func (c *Crawler) crawl(id int, jobs <-chan string, results chan<- error) {
	for randomURL := range jobs {
		// time the link getting process
		urls, pluckedData, err := c.scrapeLinks(randomURL)
		if err != nil {
			results <- errors.Wrap(err, "worker #"+strconv.Itoa(id)+" failed scraping, will retry")
			// move url to back to 'todo'
			_, err2 := c.doing.Del(randomURL).Result()
			if err2 != nil {
				c.log.Error(err2.Error())
			}
			_, err2 = c.todo.Set(randomURL, "", 0).Result()
			if err2 != nil {
				c.log.Error(err2.Error())
			}
			continue
		}

		t := time.Now()

		// move url to 'done'
		_, err = c.doing.Del(randomURL).Result()
		if err != nil {
			results <- errors.Wrap(err, "worker #"+strconv.Itoa(id))
			continue
		}
		_, err = c.done.Set(randomURL, pluckedData, 0).Result()
		if err != nil {
			results <- errors.Wrap(err, "worker #"+strconv.Itoa(id))
			continue
		}

		// add new urls to 'todo'
		for _, url := range urls {
			err = c.addLinkToDo(url, false)
			if err != nil {
				results <- errors.Wrap(err, "worker #"+string(id))
				continue
			}
		}
		c.log.Info("worker #%d: %d urls and %d bytes from %s [%s]", id, len(urls), len(pluckedData), randomURL, time.Since(t).String())
		c.numberOfURLSParsed++
		results <- nil
	}
}

func (c *Crawler) AddSeeds(seeds []string, force ...bool) (err error) {
	// add beginning link
	var bar *pb.ProgressBar
	if len(seeds) > 100 {
		fmt.Println("Adding seeds...")
		bar = pb.StartNew(len(seeds))
		defer bar.Finish()
	}
	toForce := false
	if len(force) > 0 {
		toForce = force[0]
	}
	for _, seed := range seeds {
		if len(seeds) > 100 {
			bar.Increment()
		}
		err = c.addLinkToDo(seed, toForce)
		if err != nil {
			return
		}
	}
	c.log.Info("Added %d seed links", len(seeds))
	return
}

func (c *Crawler) enqueue() {
	defer c.stopCrawling()
	for {
		time.Sleep(100 * time.Millisecond)
		// check if queue is full
		c.queue.RLock()
		queueSize := len(c.queue.Data)
		c.queue.RUnlock()
		if queueSize > c.MaxQueueSize {
			continue
		}

		// check if there are any links to do
		t := time.Now()
		dbsize, err := c.todo.DbSize().Result()
		if err != nil {
			log.Fatal(err)
		}

		// break if there are no links to do
		if dbsize == 0 && !c.workersWorking {
			c.log.Info("No more work to do!")
			break
		}

		urlsToDo := make([]string, c.MaxNumberWorkers)
		i := 0
		iter := c.todo.Scan(0, "", 0).Iterator()
		for iter.Next() {
			urlsToDo[i] = iter.Val()
			i++
			if i == len(urlsToDo) {
				break
			}
		}
		urlsToDo = urlsToDo[:i]
		if len(urlsToDo) == 0 {
			continue
		}

		// move to 'doing'
		_, err = c.todo.Del(urlsToDo...).Result()
		if err != nil {
			log.Fatal(errors.Wrap(err, "problem removing from todo"))
		}
		pairs := make([]interface{}, len(urlsToDo)*2)
		for i := 0; i < len(urlsToDo)*2; i += 2 {
			pairs[i] = urlsToDo[i/2]
			pairs[i+1] = ""
		}

		_, err = c.doing.MSet(pairs...).Result()
		if err != nil {
			log.Fatal(errors.Wrap(err, "problem placing in doing"))
		}

		if queueSize+len(urlsToDo) > 0 {
			c.log.Info("Collected %d URLs to send to workers [%s]", queueSize+len(urlsToDo), time.Since(t).String())
		}

		c.queue.Lock()
		for _, url := range urlsToDo {
			c.queue.Data[url] = struct{}{}
		}
		c.queue.Unlock()

	}
}

// Crawl initiates the pool of connections and begins
// scraping URLs according to the todo list
func (c *Crawler) Crawl() (err error) {
	fmt.Printf("\nStarting crawl on %s\n\n", c.Settings.BaseURL)
	b, _ := json.MarshalIndent(c, "", " ")
	fmt.Printf("Settings:\n%s\n\n", b)
	c.programTime = time.Now()
	c.numberOfURLSParsed = 0
	c.isRunning = true
	go c.contantlyPrintStats()
	go c.enqueue()
	for {
		if !c.isRunning {
			break
		}

		c.queue.RLock()
		queueSize := len(c.queue.Data)
		c.queue.RUnlock()

		if queueSize == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		jobs := make(chan string, queueSize)
		results := make(chan error, queueSize)

		c.workersWorking = true
		for w := 0; w < c.MaxNumberWorkers; w++ {
			go c.crawl(w, jobs, results)
		}

		num := 0
		c.queue.Lock()
		for j := range c.queue.Data {
			c.log.Trace("Adding job %s", j)
			delete(c.queue.Data, j)
			jobs <- j
			num++
			if num == c.MaxNumberWorkers {
				break
			}
		}
		c.queue.Unlock()
		close(jobs)

		for a := 0; a < c.MaxNumberWorkers; a++ {
			err := <-results
			if err != nil {
				c.log.Warn(err.Error())
			}
		}
		c.workersWorking = false
	}
	return
}

func (c *Crawler) stopCrawling() {
	c.isRunning = false
	c.printStats()
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
	fmt.Println(`                                           parsed speed   todo     done     doing   trash      errors
                                                (urls/min)`)
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
	printURL := strings.Replace(c.Settings.BaseURL, "https://", "", 1)
	printURL = strings.Replace(printURL, "http://", "", 1)
	if len(printURL) > 17 {
		printURL = printURL[:17]
	}
	log.Printf("[%17s] %9s %3d %8s %8s %8s %8s %8s\n",
		printURL,
		humanize.Comma(int64(c.numberOfURLSParsed)),
		URLSPerSecond,
		humanize.Comma(int64(c.numToDo)),
		humanize.Comma(int64(c.numDone)),
		humanize.Comma(int64(c.numDoing)),
		humanize.Comma(int64(c.numTrash)),
		humanize.Comma(int64(c.errors)))
}
