// loadgen — a tiny HTTP load generator (chapter 8.5).
//
// Two ways to generate load:
//
//	closed loop: -c workers, each sends a request, waits for the answer, repeats.
//	open loop:   -rate requests per second arrive on a schedule, whether or not
//	             earlier ones have finished — like real users.
//
// Examples:
//
//	go run ./examples/loadgen -url http://localhost:8080/healthz -c 10 -d 5s
//	go run ./examples/loadgen -url http://localhost:8080/healthz -rate 2000 -d 5s
//
// Latency is measured from when a request was *supposed* to start, so in open
// loop a slow server can't hide its queueing (see "coordinated omission").
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"
)

type result struct {
	latency time.Duration
	status  int // 0 = network error
}

func main() {
	url := flag.String("url", "http://localhost:8080/healthz", "URL to GET")
	conc := flag.Int("c", 10, "closed loop: number of concurrent workers")
	rate := flag.Int("rate", 0, "open loop: requests per second (overrides -c)")
	dur := flag.Duration("d", 5*time.Second, "how long to run")
	timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
	flag.Parse()

	client := &http.Client{
		Timeout: *timeout,
		// Don't follow redirects: we're measuring snip, not the destination site.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{MaxIdleConnsPerHost: 1000},
	}

	do := func(intended time.Time) result {
		resp, err := client.Get(*url)
		if err != nil {
			return result{latency: time.Since(intended)}
		}
		io.Copy(io.Discard, resp.Body) // read the body fully so the connection is reused
		resp.Body.Close()
		return result{latency: time.Since(intended), status: resp.StatusCode}
	}

	var (
		mu      sync.Mutex
		results []result
		wg      sync.WaitGroup
	)
	record := func(r result) { mu.Lock(); results = append(results, r); mu.Unlock() }

	start := time.Now()
	deadline := start.Add(*dur)

	if *rate > 0 {
		// Open loop: request i is due at start + i/rate, no matter what.
		interval := time.Second / time.Duration(*rate)
		for i := 0; ; i++ {
			due := start.Add(time.Duration(i) * interval)
			if due.After(deadline) {
				break
			}
			time.Sleep(time.Until(due))
			wg.Add(1)
			go func() { defer wg.Done(); record(do(due)) }()
		}
	} else {
		// Closed loop: each worker waits for its answer before sending again.
		for w := 0; w < *conc; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for time.Now().Before(deadline) {
					record(do(time.Now()))
				}
			}()
		}
	}
	wg.Wait()
	elapsed := time.Since(start)
	report(results, elapsed)
	if len(results) == 0 {
		os.Exit(1)
	}
}

func report(rs []result, elapsed time.Duration) {
	codes := map[int]int{}
	lat := make([]time.Duration, 0, len(rs))
	for _, r := range rs {
		codes[r.status]++
		lat = append(lat, r.latency)
	}
	slices.Sort(lat)
	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)]
	}
	fmt.Printf("requests:   %d in %.1fs = %.0f req/s\n", len(rs), elapsed.Seconds(), float64(len(rs))/elapsed.Seconds())
	fmt.Printf("statuses:  ")
	keys := make([]int, 0, len(codes))
	for k := range codes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		name := fmt.Sprint(k)
		if k == 0 {
			name = "error"
		}
		fmt.Printf(" %s×%d", name, codes[k])
	}
	fmt.Println()
	fmt.Printf("latency:    p50 %v   p90 %v   p99 %v   max %v\n",
		pct(0.50).Round(time.Microsecond), pct(0.90).Round(time.Microsecond),
		pct(0.99).Round(time.Microsecond), pct(1).Round(time.Microsecond))
}
