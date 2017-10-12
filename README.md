<img src="https://user-images.githubusercontent.com/6550035/31456157-58663efe-ae76-11e7-8e53-6a2a5b7a196c.png" width="450" border="0" alt="crawdad" style="float:right;" align="right">
<h1>crawdad</h1>
<p>
<a href="https://github.com/schollz/crawdad/releases/latest"><img src="https://img.shields.io/badge/version-3.0.0-brightgreen.svg?style=flat-square" alt="Version"></a>&nbsp;<img src="https://img.shields.io/badge/coverage-59%25-yellow.svg?style=flat-square" alt="Code Coverage">
<br>
<em>crawdad</em> is cross-platform web-crawler that can also pinch data. <em>crawdad</em> is persistent, distributed, and fast. It uses a queue stored in a remote Redis database to persist after interruptions and also synchronize distributed instances. Data extraction can be specified by the simple and powerful <a href="https://github.com/schollz/pluck"><em>pluck</em></a> syntax. 
</p>

Crawl responsibly.

For a tutorial on how to use *crawdad* see [my blog post](https://schollz.github.io/crawdad/).

# Features

- Written in Go
- [Cross-platform releases](https://github.com/schollz/crawdad/releases/latest)
- Persistent (interruptions can be re-initialized)
- Distributed (multiple crawdads can be run on diferent machines)
- Scraping using [*pluck*](https://github.com/schollz/pluck)
- Uses connection pools for lower latency
- Uses threads for maximum parallelism

# Install

First [get Docker CE](https://www.docker.com/community-edition). This will make installing Redis a snap.

Then, if you have Go installed, just do

```
$ go get github.com/schollz/crawdad
```

Otherwise, use the releases and [download crawdad](https://github.com/schollz/crawdad/releases/latest).

# Run

First run Redis:

```sh
$ docker run -d -v `pwd`:/data -p 6379:6379 redis 
```

which will store the database in the current directory.

## Crawling 

By "crawling* the *crawdad* will follow every link that corresponds to the base URL. This is useful for generating sitemaps.

Startup *crawdad* with the base URL:

```sh
$ crawdad -set -url https://rpiai.com
```

This command will set the base URL to crawl as `https://rpiai.com`. You can run *crawdad* on a different machine without setting these parameters again. E.g., on computer 2 you can run:

```sh
$ crawdad -server X.X.X.X
```

where `X.X.X.X` is the IP address of computer 2. This crawdad will now run with whatever parameters set from the first one. If you need to re-set parameters, just use `-set` to specify them again.

Each machine running *crawdad* will help to crawl the respective website and add collected links to a universal queue in the server. The current state of the crawler is saved. If the crawler is interrupted, you can simply run the command again and it will restart from the last state.

When done you can dump all the links:

```sh
$ crawdad -dump dump.txt
```

which will connect to Redis and dump all the links to-do, doing, done, and trashed.

## Pinching

By "pinching" the *crawdad* will follow the specified links and extract data from each URL that can be dumped later.

You will need to make a [*pluck* TOML configuration file](https://github.com/schollz/pluck). For instance, I would like to scrape from my site, rpiai.com, the meta description and the title. My configuration, `pluck.toml`, looks like:

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


Now I can crawl the site the same way as before, but load in this *pluck* configuration with `--pluck` so it captures the content:

```sh
$ crawdad -set -url "https://rpiai.com" -pluck pluck.toml
```

To retrieve the data, then you can use the `-done` flag to collect a JSON map of all the plucked data.

```sh
$ crawdad -done data.json
```

This data JSON file will contain each URL as a key and a JSON string of the finished data that contain keys for the description and the title.

```sh
$ cat data.json | grep why
"https://rpiai.com/why-i-made-a-book-recommendation-service/index.html": "{\"description\":\"Why I made a book recommendation service from scratch: basically I found that all other book suggestions lacked so I made something that actually worked.\",\"title\":\"What book is similar to Weaveworld by Clive Barker?\"}"
```

# Advanced usage

There are lots of other options:

```
   --server value, -s value       address for Redis server (default: "localhost")
   --port value, -p value         port for Redis server (default: "6379")
   --url value, -u value          set base URL to crawl
   --exclude value, -e value      set comma-delimted phrases that must NOT be in URL
   --include value, -i value      set comma-delimted phrases that must be in URL
   --seed file                    file with URLs to add to queue
   --pluck value                  set config file for a plucker (see github.com/schollz/pluck)
   --stats X                      Print stats every X seconds (default: 1)
   --connections value, -c value  number of connections to use (default: 25)
   --workers value, -w value      number of connections to use (default: 8)
   --verbose                      turn on logging
   --proxy                        use tor proxy
   --set                          set options across crawdads
   --dump file                    dump all the keys to file
   --done file                    dump the map of the done things file
   --useragent useragent          set the specified useragent
   --redo                         move items from 'doing' to 'todo'
   --query                        allow query parameters in URL
   --hash                         allow hashes in URL
   --no-follow                    do not follow links (useful with -seed)
   --errors value                 maximum number of errors before exiting (default: 10)
   --help, -h                     show help
   --version, -v                  print the version

```

# Dev

To run tests

```
$ docker run -d -v `pwd`:/data -p 6379:6379 redis
$ cd crawdad && go test -v -cover
```

# License

MIT
