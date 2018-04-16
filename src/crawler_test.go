package crawdad

import (
	"fmt"
	"testing"
)

func TestGeneral(t *testing.T) {

	crawl, err := New()
	if err != nil {
		t.Error(err)
	}

	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6379"

	err = crawl.Init(Settings{
		BaseURL: "http://rpiai.com",
	})
	if err != nil {
		t.Error(err)
	}

	urls, _, err := crawl.scrapeLinks("http://rpiai.com")
	if err != nil {
		t.Error(err)
	}
	if len(urls) < 15 {
		t.Error("%v", urls)
	}

	err = crawl.Crawl()
	if err != nil {
		t.Error(err)
	}
	err = crawl.Flush()
	if err != nil {
		t.Error(err)
	}
}

func TestProxy(t *testing.T) {
	crawl, err := New()
	if err != nil {
		t.Error(err)
	}
	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6379"

	err = crawl.Init(Settings{
		BaseURL: "http://rpiai.com",
	})
	if err != nil {
		t.Error(err)
	}
	ip, err := crawl.getIP()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(ip)
	urls, _, err := crawl.scrapeLinks("http://rpiai.com")
	if err != nil {
		t.Error(err)
	}
	if len(urls) < 15 {
		t.Error("%v", urls)
	}
}

func TestPlucking(t *testing.T) {

	crawl, err := New()
	if err != nil {
		t.Error(err)
	}
	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6379"

	err = crawl.Init(Settings{
		BaseURL: "https://rpiai.com",
		PluckConfig: `[[pluck]]
activators = ['h1','"post-full-title','>']
deactivator = '</'
limit = 1`,
	})
	if err != nil {
		t.Error(err)
	}
	err = crawl.Crawl()
	if err != nil {
		t.Error(err)
	}

	fmt.Println(crawl.DumpMap())
	err = crawl.Flush()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(crawl.DumpMap())
}
