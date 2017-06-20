package crawler

import (
	"fmt"
	"testing"
)

func TestGeneral(t *testing.T) {

	crawl, err := New("http://rpiai.com/", true, false)
	if err != nil {
		t.Error(err)
	}
	urls, err := crawl.scrapeLinks("http://rpiai.com")
	if err != nil {
		t.Error(err)
	}
	if len(urls) < 15 {
		t.Error("%v", urls)
	}
}

func TestCrawl(t *testing.T) {

	crawl, err := New("http://rpiai.com/", true, false)
	if err != nil {
		t.Error(err)
	}
	err = crawl.Crawl()
	if err != nil {
		t.Error(err)
	}
}

func TestProxy(t *testing.T) {
	crawl, err := New("http://rpiai.com/", true, true)
	if err != nil {
		t.Error(err)
	}
	ip, err := crawl.getIP()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(ip)
	urls, err := crawl.scrapeLinks("http://rpiai.com")
	if err != nil {
		t.Error(err)
	}
	if len(urls) < 15 {
		t.Error("%v", urls)
	}
}
