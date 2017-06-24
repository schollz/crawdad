# goredis-crawler

Go+Redis for cross-platform persistent and distributed web crawler

*goredis-crawler* is persistent because the queue is stored in a remote database that is automatically re-initialized if interrupted. *goredis-crawler* is distributed because multiple instances of *goredis-crawler* will work on the remotely stored queue, so you can start as many crawlers as you want on separate machines to speed along the process. *goredis-crawler* is also fast because it is threaded and uses connection pools.

Crawl responsibly.

Getting Started
===============

## Install

If you have Go installed, just do
```
go get github.com/schollz/goredis-crawler/...
```

Otherwise, use the releases and [download goredis-crawler](https://github.com/schollz/goredis-crawler/releases/latest).

## Run

### Crawl a site

First run the database server which will create a LAN hub:

```sh
$ docker run -d -v /place/to/save/data:/data -p 6379:6379 redis 
$ ./goredis-crawler --url "http://rpiai.com"
```
You can run this last command on as many different machines as you want, which will help to crawl the respective website and add collected links to a universal queue in the server.

The current state of the crawler is saved. If the crawler is interrupted, you can simply run the command again and it will restart from the last state.

See the help (`-help`) if you'd like to see more options, such as exclusions/inclusions and modifying the worker pool and connection pools.

## License

MIT
