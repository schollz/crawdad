
<p align="center">
<img
    src="https://user-images.githubusercontent.com/6550035/30241126-96b2b5f2-953a-11e7-8159-bc276ab87201.png"
    width="260" height="80" border="0" alt="pluck">
<br>
<a href="https://github.com/schollz/crab/releases/latest"><img src="https://img.shields.io/badge/version-2.0.0-brightgreen.svg?style=flat-square" alt="Version"></a>
<img src="https://img.shields.io/badge/coverage-59%25-yellow.svg?style=flat-square" alt="Code Coverage">
</p>

<p align="center">*crab* is cross-platform crawler that can also pinch things on the web.</p>

*crab* is a web-crawler and scraper that is persistent, distributed, and fast. It uses a queue stored in a remote Redis database to synchronize distributed *crab*s and allows scraping to be specified in a flexible way using [*pluck*](https://githbu.com/schollz/pluck). Use *crab* to crawl an entire domain and scrape selected content.

Crawl responsibly.

# Features

- Written in Go
- [Cross-platform releases](https://github.com/schollz/crab/releases/latest)
- Persistent (interruptions can be re-initialized)
- Distributed (multiple crabs can be run on diferent machiness)
- Scraping using [*pluck*](https://github.com/schollz/pluck)
- Uses connection pools for lower latency
- Uses threads for maximum parallelism

# Install

First [get Docker CE](https://www.docker.com/community-edition). This will make installing Redis a snap.

Then, if you have Go installed, just do

```
$ go get github.com/schollz/crab/...
```

Otherwise, use the releases and [download crab](https://github.com/schollz/crab/releases/latest).

# Run

First run Redis:

```sh
$ docker run -d -v /place/to/save/data:/data -p 6378:6379 redis 
```

## Crawling 

Feel free to change the port on your computer (`6378`) to whatever you want. Then startup *crab* with the base URL and the Redis port:

```sh
$ crab --port 6378 --url "http://rpiai.com"
```

To run on different machines, just specify the Redis server address with `--server`. Make sure to forward the port on the Redis machine. Then on a different machine, just run:

```sh
$ crab --server X.X.X.X --port 6378 --url "http://rpiai.com"
```

Each machine running *crab* will help to crawl the respective website and add collected links to a universal queue in the server. The current state of the crawler is saved. If the crawler is interrupted, you can simply run the command again and it will restart from the last state.

When done you can dump all the links:

```sh
$ crab --port 6378 --dump dump.txt
```

which will connect to Redis and dump all the links to-do, doing, done, and trashed.

## Scraping

To scrape, you will need to make a [*pluck* TOML](https://github.com/schollz/pluck). For instance, I would like to scrape from my site, rpiai.com, the meta description and the title. My TOML file, `pluck.toml`, looks like:

```toml
[[pluck]]
name = "description"
activators = ["meta","name","description",'content="']
deactivator = '"'
limit = 1

[[pluck]]
name = "title"
activators = ["<title>"]
deactivator = "</title>"
limit = 1
```

Now I can crawl the site the same way as before, but load in this *pluck* TOML so it captures the content:

```sh
$ crab --port 6378 --url "https://rpiai.com" --pluck pluck.toml
```

To retrieve the data, then you can use the `--done` flag to collect a JSON map of all the plucked data.

```sh
$ crab --port 6378 --done data.json
```

This data JSON file will contain each URL as a key and a JSON string of the finished data that contain keys for the description and the title.

```sh
$ cat data.json | grep why
"https://rpiai.com/why-i-made-a-book-recommendation-service/index.html": "{\"description\":\"Why I made a book recommendation service from scratch: basically I found that all other book suggestions lacked so I made something that actually worked.\",\"title\":\"What book is similar to Weaveworld by Clive Barker?\"}"
```

# Advanced usage

There are lots of other options:

```
   --url value, -u value          base URL to crawl
   --seed value                   URL to seed with
   --server value, -s value       address for Redis server (default: "localhost")
   --port value, -p value         port for Redis server (default: "6379")
   --exclude value, -e value      comma-delimted phrases that must NOT be in URL
   --include value, -i value      comma-delimted phrases that must be in URL
   --pluck value                  config file for a plucker (see github.com/schollz/pluck)
   --stats X                      Print stats every X seconds (default: 1)
   --connections value, -c value  number of connections to use (default: 25)
   --workers value, -w value      number of connections to use (default: 8)
   --verbose                      turn on logging
   --proxy                        use tor proxy
   --dump file                    dump all the keys to file
   --done file                    dump the map of the done things file
   --useragent useragent          set the specified useragent
   --redo                         move items from 'doing' to 'todo'
   --query                        allow query parameters in URL
   --hash                         allow hashes in URL
   --errors value                 maximum number of errors before exiting (default: 10)
   --help, -h                     show help
   --version, -v                  print the version
```

# Dev

To run tests

```
$ docker run -d -v `pwd`:/data -p 6379:6379 redis
$ cd lib && go test -cover
```

# License

MIT
