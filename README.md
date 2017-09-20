
<p align="center">
<img
    src="https://user-images.githubusercontent.com/6550035/30241126-96b2b5f2-953a-11e7-8159-bc276ab87201.png"
    width="260" height="80" border="0" alt="pluck">
<br>
<a href="https://github.com/schollz/goredis-crawler/releases/latest"><img src="https://img.shields.io/badge/version-1.1.0-brightgreen.svg?style=flat-square" alt="Version"></a>
<img src="https://img.shields.io/badge/coverage-59%25-yellow.svg?style=flat-square" alt="Code Coverage">
</p>

<p align="center">A cross-platform persistent and distributed web crawler.</p>

*goredis-crawler* is persistent because the queue is stored in a remote database that is automatically re-initialized if interrupted. *goredis-crawler* is distributed because multiple instances of *goredis-crawler* will work on the remotely stored queue, so you can start as many crawlers as you want on separate machines to speed along the process. *goredis-crawler* is also fast because it is threaded and uses connection pools.

Crawl responsibly.

# Install

First [get Docker CE](https://www.docker.com/community-edition). This will make installing Redis a snap.

Then, if you have Go installed, just do

```
$ go get github.com/schollz/goredis-crawler/...
```

Otherwise, use the releases and [download goredis-crawler](https://github.com/schollz/goredis-crawler/releases/latest).

# Run

First run Redis:

```sh
$ docker run -d -v /place/to/save/data:/data -p 6378:6379 redis 
```

Feel free to change the port on your computer (`6378`) to whatever you want. Then startup *goredis-crawler* with the base URL and the Redis port:

```sh
$ goredis-crawler --port 6378 --url "http://rpiai.com"
```

You can run this last command on as many different machines as you want, just make sure to specify the Redis server address with `--server`. Each machine running *goredis-crawler* will help to crawl the respective website and add collected links to a universal queue in the server. The current state of the crawler is saved. If the crawler is interrupted, you can simply run the command again and it will restart from the last state.

When done you can dump all the links:

```sh
$ goredis-crawler --port 6378 --dump dump.txt
```

which will connect to Redis and dump all the links to-do, doing, done, and trashed.

# Advanced usage

There are lots of other options:

```
   --url value, -u value          base URL to crawl
   --seed value                   URL to seed with
   --server value, -s value       address for Redis server (default: "localhost")
   --port value, -p value         port for Redis server (default: "6379")
   --exclude value, -e value      comma-delimted phrases that must NOT be in URL
   --include value, -i value      comma-delimted phrases that must be in URL
   --stats X                      Print stats every X seconds (default: 1)
   --connections value, -c value  number of connections to use (default: 25)
   --workers value, -w value      number of connections to use (default: 8)
   --verbose                      turn on logging
   --proxy                        use tor proxy
   --dump file                    dump the records to file
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
