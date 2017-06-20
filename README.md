
<p align="center">
<h2>goredis-crawler</h2>
<br>
<a href="http://gocover.io/github.com/schollz/goredis-crawler/lib"><img src="https://img.shields.io/badge/coverage-76%25-yellow.svg?style=flat-square" alt="Code Coverage"></a>
<a href="https://godoc.org/github.com/schollz/goredis-crawler/lib"><img src="https://img.shields.io/badge/api-reference-blue.svg?style=flat-square" alt="GoDoc"></a>
</p>

<p align="center">Cross-platform persistent and distributed web crawler</a></p>

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
