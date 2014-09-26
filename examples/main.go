package main

import (
	"fmt"
	"net/http"

	"github.com/bradleyg/go-limit"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})

	limits := &golimit.Limits{
		golimit.Limit{"GET", "/", 3, 60},
	}

	limiter := golimit.NewLimiter(limits, nil, nil)

	http.ListenAndServe(":8080", limiter.Handle(mux))
}
