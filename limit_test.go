package golimit

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/bradleyg/go-redisify"
)

var (
	limiter *Limiter
	req     *http.Request
)

func init() {
	client, err := goredisify.Conn(nil)
	if err != nil {
		log.Fatal(err)
	}

	keys, err := client.Keys("go-ratelimit:*")
	if err != nil {
		log.Fatal(err)
	}

	for _, k := range keys {
		client.Del(k)
	}

	limits := &Limits{
		Limit{
			Method:   "GET",
			Path:     "/test",
			Requests: 5,
			Duration: 30,
		},
		Limit{
			Method:   "GET",
			Path:     "/expire",
			Requests: 5,
			Duration: 1,
		},
	}

	limiter = NewLimiter(limits, nil, nil)

	r, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		log.Fatal(err)
	}

	req = r
}

func testHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
	fmt.Fprint(rw, "ok")
}

func TestRateLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", testHandler)

	h := limiter.Handle(mux)
	srv := httptest.NewServer(h)

	var i int64
	limit := limiter.LimitsMap["GET:/test"].Requests
	duration := limiter.LimitsMap["GET:/test"].Duration

	for i = 0; i <= limit; i++ {

		res, err := http.Get(srv.URL + "/test")
		if err != nil {
			t.Fatal(err)
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}

		res.Body.Close()

		remaining, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Remaining"), 10, 64)
		if err != nil {
			t.Fatalf("Could not parse int from X-RateLimit-Remaining", err)
		}

		expectedR := limit - i - 1
		if i == limit {
			expectedR++
		}

		if remaining != expectedR {
			t.Fatalf("Remaining count incorrect. Expected %d, Actual %d", remaining, expectedR)
		}

		if i == limit {
			if res.StatusCode != 429 {
				t.Fatalf("Incorrect status code when limiting. Expected %d, Actual %d", 429, res.StatusCode)
			}

			if string(body) != "429, Too Many Requests" {
				t.Fatalf("Body should be empty when rate limiting. Expected %s, Actual %s", "429, Too Many Requests", string(body))
			}

			retryAfter, err := strconv.ParseInt(res.Header.Get("Retry-After"), 10, 64)
			if err != nil {
				t.Fatalf("Could not parse int from Retry-After", err)
			}

			if retryAfter != duration {
				t.Fatalf("Remaining count incorrect. Expected %d, Actual %d", duration, retryAfter)
			}
		} else {
			if res.StatusCode != http.StatusOK {
				t.Fatalf("Incorrect status code returned when not limiting. Expected %d, Actual %d", http.StatusOK, res.StatusCode)
			}

			if string(body) != "ok" {
				t.Fatalf("Incorrect body when not limiting. Expect %s, Actual %s", "ok", string(body))
			}
		}

	}
}

func TestRateLimitExpire(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/expire", testHandler)

	h := limiter.Handle(mux)
	srv := httptest.NewServer(h)

	var i int64
	limit := limiter.LimitsMap["GET:/expire"].Requests

	for i = 0; i <= limit+1; i++ {
		res, err := http.Get(srv.URL + "/expire")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		if i == limit && res.StatusCode != 429 {
			t.Fatalf("Incorrect status code when limiting. Expected %d, Actual %d", 429, res.StatusCode)
		}

		if i == limit+1 && res.StatusCode != 200 {
			t.Fatalf("Incorrect status code when limiting. Expected %d, Actual %d", 200, res.StatusCode)
		}

		if i == limit {
			time.Sleep(1 * time.Second)
		}
	}
}
