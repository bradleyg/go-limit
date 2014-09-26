// Package golimit limits http requests storing requests in redis.
package golimit

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/hoisie/redis"
)

// A Limit defines the necessary values to limit a request.
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

// Limits holds a slice of Limit to allow multiple limited routes.
type Limits []Limit

// A Limiter is returned by NewLimiter.
type Limiter struct {
	// Header specifies the ip proxy header to look for to limit requests.
	// For example Heroku uses X-FORWARDED-FOR. To look for the remote address
	// rather than a proxy header use an empty string.
	Header string
	// LimitsMap contains a map using method+path to speed up lookups.
	LimitsMap limitsMap
}

type limitsMap map[string]Limit

var (
	client  *redis.Client
	logErr  = log.New(os.Stderr, "[go-limit:error] ", 0)
	logInfo = log.New(os.Stdout, "[go-limit:info] ", 0)
)

func ipAddrFromRemoteAddr(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return s
	}
	return s[:idx]
}

func getAddress(r *http.Request, header string) (string, error) {
	var headerVal string

	if header == "" {
		headerVal = r.RemoteAddr
	} else {
		headerVal = r.Header.Get(header)
	}

	log.Println("-----")
	log.Println(header)

	addresses := strings.Split(headerVal, ",")
	address := strings.TrimSpace(addresses[0])
	address = ipAddrFromRemoteAddr(address)

	if address == "" {
		err := errors.New("Could not read address")
		return "", err
	}

	return address, nil
}

func expire(count int64, key string, duration int64) error {
	if count == 1 {
		_, err := client.Expire(key, duration)
		if err != nil {
			return err
		}
	}
	return nil
}

func redisConn() *redis.Client {
	redisURL := os.Getenv("REDIS_URL")

	parsed, err := url.Parse(redisURL)
	if err != nil {
		logErr.Panic(err)
	}

	c := redis.Client{
		Addr: parsed.Host,
	}

	if parsed.User != nil {
		password, ok := parsed.User.Password()
		if ok {
			c.Password = password
		}
	}

	return &c
}

func setHeaders(rw http.ResponseWriter, limit Limit, count int64, timeout int64) {
	remaining := limit.Requests - count
	if remaining < 0 {
		remaining = 0
	}

	rw.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	rw.Header().Set("X-RateLimit-Limit", strconv.FormatInt(limit.Requests, 10))

	if timeout >= 0 {
		rw.Header().Set("Retry-After", strconv.FormatInt(timeout, 10))
	}
}

// Creates a new rate limiter. If you already have a redis connection available
// via github.com/hoisie/redis you can pass it as the last parameter. Passing nil
// will create a new redis connection (The enviroment variable "REDIS_URL" must be set).
func NewLimiter(limits *Limits, header string, c *redis.Client) *Limiter {
	lMap := make(limitsMap)

	if c == nil {
		client = redisConn()
	} else {
		client = c
	}

	for _, limit := range *limits {
		key := limit.Method + ":" + limit.Path
		lMap[key] = limit
	}

	return &Limiter{header, lMap}
}

// Handler takes and returns a http.Handler. Best used as a middleware chain.
// Eg.
//
// mux := http.NewServeMux()
// mux.HandleFunc("/", ...)
//
// limiter := golimit.NewLimiter(...)
// http.ListenAndServe(":80", limiter.Handle(mux))
//
func (l Limiter) Handle(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		lMap := l.LimitsMap

		limit, ok := lMap[r.Method+":"+r.URL.Path]
		if !ok {
			handler.ServeHTTP(rw, r)
			return
		}

		address, err := getAddress(r, l.Header)
		if err != nil {
			logErr.Println(err)
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		key := "go-ratelimit:(" + address + ")" + r.Method + r.URL.Path

		count, err := client.Incr(key)
		if err != nil {
			logErr.Println(err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = expire(count, key, limit.Duration); err != nil {
			logErr.Println(err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		if count > limit.Requests {
			timeout, err := client.Ttl(key)
			if err != nil {
				logErr.Println(err)
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}

			logInfo.Println("Limiting " + key)

			setHeaders(rw, limit, count, timeout)
			rw.WriteHeader(429)
			return
		}

		setHeaders(rw, limit, count, -1)
		handler.ServeHTTP(rw, r)
	})
}
