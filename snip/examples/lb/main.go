// lb — a round-robin load balancer in about 40 lines (chapter 8.6).
//
// It forwards each incoming request to the next backend in turn, the same idea
// as nginx's upstream block (chapter 8.3), so you can watch horizontal scaling
// without installing anything.
//
//	go run ./examples/lb -listen :8000 http://localhost:9091 http://localhost:9092
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
)

func main() {
	listen := flag.String("listen", ":8000", "address to listen on")
	flag.Parse()
	if flag.NArg() == 0 {
		log.Fatal("usage: lb [-listen :8000] BACKEND_URL...")
	}

	var proxies []*httputil.ReverseProxy
	for _, raw := range flag.Args() {
		u, err := url.Parse(raw)
		if err != nil {
			log.Fatalf("bad backend %q: %v", raw, err)
		}
		p := httputil.NewSingleHostReverseProxy(u)
		p.Transport = &http.Transport{MaxIdleConnsPerHost: 1000} // reuse connections to backends
		proxies = append(proxies, p)
	}

	var next atomic.Uint64 // shared by all requests, so it must be safe for concurrent use
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i := next.Add(1) % uint64(len(proxies)) // round robin: 1, 2, 1, 2, …
		proxies[i].ServeHTTP(w, r)
	})
	log.Printf("lb on %s → %v", *listen, flag.Args())
	log.Fatal(http.ListenAndServe(*listen, nil))
}
