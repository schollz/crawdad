package crawler

import (
	"testing"
)

func TestGeneral(t *testing.T) {

	crawl, err := New("http://rpiai.com/")
	if err != nil {
		t.Error(err)
	}

	crawl.RedisURL = "192.168.0.17"
	crawl.RedisPort = "6377"
	crawl.Verbose = true

	err = crawl.Init()
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

	err = crawl.Crawl()
	if err != nil {
		t.Error(err)
	}
}

// func TestProxy(t *testing.T) {
// 	crawl, err := New("http://rpiai.com/")
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	ip, err := crawl.getIP()
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	fmt.Println(ip)
// 	urls, err := crawl.scrapeLinks("http://rpiai.com")
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	if len(urls) < 15 {
// 		t.Error("%v", urls)
// 	}
// }
