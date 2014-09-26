# golimit
--
    import "github.com/bradleyg/go-limit"

Package golimit limits http requests storing requests in redis.

## Usage

#### type Limit

```go
type Limit struct {
	// Method specifies what type of http method you want to rate limit.
	Method string
	// Path Specifies the path (r.URL.Path) of the which http requests to limit.
	Path string
	// Requests specifies how many requests are allowed before limiting begins.
	Requests int64
	// Duration specifies the rate limit window in seconds.
	Duration int64
}
```

A Limit defines the necessary values to limit a request.

#### type Limiter

```go
type Limiter struct {
	// Header specifies the ip proxy header to look for to limit requests.
	// For example Heroku uses X-FORWARDED-FOR. To look for the remote address
	// rather than a proxy header use "nil".
	Header interface{}
	// LimitsMap contains a map using method+path to speed up lookups.
	LimitsMap limitsMap
}
```

A Limiter is returned by NewLimiter.

#### func  NewLimiter

```go
func NewLimiter(limits *Limits, header interface{}, c *redis.Client) *Limiter
```
Creates a new rate limiter.

By passing "nil" as the "header" argument you are asking to read the IP from
r.RemoteAddr.

    limiter := NewLimiter(limits, nil, nil)

You can also pass a string rather than nil to specify to look at a header rather
than the remote address. This is useful for when serving requests behind a
proxy. For example Heroku passes through the remote IP in the header
"X-Forwarded-For".

    limiter := NewLimiter(limits, "X-Forwarded-For", nil)

If you already have a redis connection available via github.com/hoisie/redis you
can pass it as the last parameter. Passing nil will create a new redis
connection. The default connection will user localhost but the enviroment
variable "REDIS_URL" can also be set and used.

    limiter := NewLimiter(limits, "X-Forwarded-For", &client)

#### func (Limiter) Handle

```go
func (l Limiter) Handle(handler http.Handler) http.Handler
```
Handler takes and returns a http.Handler. Best used as a middleware chain.

    mux := http.NewServeMux()
    mux.HandleFunc("/", ...)

    limiter := golimit.NewLimiter(...)
    http.ListenAndServe(":80", limiter.Handle(mux))

#### type Limits

```go
type Limits []Limit
```

Limits holds a slice of Limit to allow multiple limited routes.
