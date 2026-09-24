// Partial failure, timeouts, retries and idempotency (chapter 8.1).
//
// Starts a deliberately unreliable "payments" server in the background, then
// sends 20 charges to it in one of two ways:
//
//	go run ./examples/retry -mode naive   # no timeout, no retry
//	go run ./examples/retry -mode retry   # timeout + retries, but NO idempotency key
//	go run ./examples/retry -mode smart   # timeout + retry with backoff & jitter + idempotency key
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"time"
)

func main() {
	mode := flag.String("mode", "smart", "naive, retry or smart")
	flag.Parse()

	addr := startFlakyServer()
	client := &http.Client{}
	if *mode != "naive" {
		client.Timeout = 300 * time.Millisecond // never wait forever
	}

	start := time.Now()
	ok, failed := 0, 0
	for i := 1; i <= 20; i++ {
		key := fmt.Sprintf("charge-%d", i) // the same key on every retry of THIS charge
		var err error
		switch *mode {
		case "smart":
			err = retry(func() error { return charge(client, addr, key) })
		case "retry":
			err = retry(func() error { return charge(client, addr, "") }) // no key: server can't spot repeats
		default:
			err = charge(client, addr, key)
		}
		if err != nil {
			failed++
		} else {
			ok++
		}
	}
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("mode=%s: %d succeeded, %d failed, took %v\n", *mode, ok, failed, time.Since(start).Round(time.Millisecond))
	fmt.Printf("server actually charged %d times for 20 intended charges\n", totalCharged)
}

func charge(c *http.Client, addr, key string) error {
	req, _ := http.NewRequest("POST", "http://"+addr+"/charge", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, err := c.Do(req)
	if err != nil {
		return err // timeout, connection reset…
	}
	res.Body.Close()
	if res.StatusCode >= 500 {
		return fmt.Errorf("server error %d", res.StatusCode)
	}
	return nil
}

// retry tries up to 5 times, waiting 50ms, 100ms, 200ms… plus random jitter,
// so many clients retrying at once don't all hit the server at the same moment.
func retry(op func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		backoff := time.Duration(50<<attempt) * time.Millisecond
		jitter := time.Duration(rand.IntN(50)) * time.Millisecond
		time.Sleep(backoff + jitter)
	}
	return err
}

// ---- the unreliable server ----

var (
	mu           sync.Mutex
	charged      = map[string]bool{} // idempotency keys already processed
	totalCharged int                 // how many times money actually moved
)

func startFlakyServer() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // any free port
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charge", func(w http.ResponseWriter, r *http.Request) {
		switch n := rand.IntN(10); {
		case n < 2: // 20%: fail before doing anything
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		case n < 4: // 20%: do the work, but the response is slow (lost to the client's timeout)
			process(r.Header.Get("Idempotency-Key"))
			time.Sleep(time.Second)
			return
		}
		process(r.Header.Get("Idempotency-Key"))
	})
	go func() { _ = http.Serve(ln, mux) }()
	return ln.Addr().String()
}

// process charges once per idempotency key, however many times it's asked.
func process(key string) {
	mu.Lock()
	defer mu.Unlock()
	if key != "" && charged[key] {
		return // already done: a retry, not a new charge
	}
	charged[key] = true
	totalCharged++
}
