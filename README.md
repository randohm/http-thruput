# http-thruput

Server and client to generate bandwidth usage over HTTP.
The goal is to have a small standalone binary that can serve several GBs of dummy data to multiple concurrent clients.
Memory usage is kept low, but CPU had to be sacrificed to manage that.

## Options
```
  -host string
    	Hostname of server running http-thruput (default "localhost")
  -mode string
    	Run in 'client' or 'server' mode. (default "server")
  -parallel
    	Run client in parallel or series (default true)
  -port int
    	Remote server port (default 8080)
  -run.get
    	Run GET test. (default true)
  -run.post
    	Run POST test. (default true)
  -segment.size int
    	Segment size for in-memory file chunks (default 10485760)
  -size string
    	Size of data. 1g, 2M, 4k, etc. Overrides -size.post and -size.get.
  -size.get string
    	Size of get data. 1g, 2M, 4k, etc. (default "1g")
  -size.post string
    	Size of post data. 1g, 2M, 4k, etc. (default "1g")
  -web.listen-address string
    	Listen address for HTTP requests (default "0.0.0.0:8080")
```

## Server Mode

This is the default mode.
There are 2 valid URLs: `/up` and `/down`.

### Download Tests

__URL:__ `http://<hostname>:<port>/down?s=<bytes>`

hostname: The hostname of the remote server running `http-thruput` in server mode.
port: The TCP port number.
bytes: The number of bytes to present to the client.

#### The 's' Parameter

The `s` parameter represents a number of bytes.
It is an integer with an optional suffix.
Allowed suffixes are: b, B, k, K, m, M, g, G.
`B` and `b` are identical and are for bytes.
For `[kKmMgG]`, the upper-case variant represents the ISO units (thousands) and lower-case for the IEC units (2 to the power of 10s)

__Examples:__
- `12345`: literally 12,345 bytes
- `128m`: 128 MiB, 128 x 2<sup>20</sup>
- `100K`: 100,000 bytes
- `8g`: 8 GiB, 8 x 2<sup>30</sup>


### Upload Tests

__URL:__ `http://<hostname>:<port>/up`

Accepts only only POSTs.
The client can post the dummy data as the body of the request.

## Client Mode

Example:
```
http-thruput -mode client -host remotehost -port 80 -size 10g
```
This will run tests against `http://remotehost:80/` using 10 GiB of dummy data in parallel.
The download and upload tests (GET and PUT, respectively) can be run in series or parallel.

Run in series:
```
http-thruput -mode client -parallel=0
```

Run get/download test only:
```
http-thruput -mode client -run.post=0
```

Run post/upload test only:
```
http-thruput -mode client -run.get=0
```

