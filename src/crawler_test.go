package crawdad

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneral(t *testing.T) {

	crawl, err := New()
	if err != nil {
		t.Error(err)
	}

	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6377"

	err = crawl.Init(Settings{
		BaseURL: "http://rpiai.com",
	})
	if err != nil {
		t.Error(err)
	}

	urls, _, err := crawl.scrapeLinks("http://rpiai.com")
	assert.Nil(t, err)
	assert.Equal(t, true, len(urls) > 15)

	err = crawl.Crawl()
	assert.Nil(t, err)
	err = crawl.Flush()
	assert.Nil(t, err)
}

func TestProxy(t *testing.T) {
	crawl, err := New()
	assert.Nil(t, err)
	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6377"

	err = crawl.Init(Settings{
		BaseURL: "http://rpiai.com",
	})
	assert.Nil(t, err)
	ip, err := crawl.getIP()
	assert.Nil(t, err)

	fmt.Println(ip)
	urls, _, err := crawl.scrapeLinks("http://rpiai.com")
	assert.Nil(t, err)
	assert.Equal(t, true, len(urls) > 15)
}

func TestPlucking(t *testing.T) {

	crawl, err := New()
	assert.Nil(t, err)
	crawl.RedisURL = "localhost"
	crawl.RedisPort = "6377"

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
