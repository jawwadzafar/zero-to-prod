// Goroutines, channels, a worker pool and context timeouts (chapter 4.5).
//
//	go run ./examples/workers                 # 3 workers share 10 jobs
//	go run ./examples/workers -workers 10     # more workers, faster
//	go run ./examples/workers -timeout 500ms  # give up early: context cancellation
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

type result struct {
	job    int
	worker int
	took   time.Duration
}

func main() {
	workers := flag.Int("workers", 3, "how many goroutines process jobs")
	timeout := flag.Duration("timeout", 5*time.Second, "give up after this long")
	flag.Parse()

	// A context carries a deadline: when it passes, ctx.Done() is closed.
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	jobs := make(chan int)       // unbuffered channel: work handed from main to workers
	results := make(chan result) // results handed back from workers to main

	var wg sync.WaitGroup
	for w := 1; w <= *workers; w++ {
		wg.Add(1)
		go func() { // each worker is a goroutine
			defer wg.Done()
			for job := range jobs { // take jobs until the channel is closed
				took, err := slowWork(ctx)
				if err != nil {
					return // cancelled: stop working
				}
				results <- result{job: job, worker: w, took: took}
			}
		}()
	}

	// Feed the jobs in their own goroutine, then close the channel: "no more work".
	go func() {
		defer close(jobs)
		for j := 1; j <= 10; j++ {
			select {
			case jobs <- j:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Close results once every worker has finished, so the loop below ends.
	go func() {
		wg.Wait()
		close(results)
	}()

	start := time.Now()
	done := 0
	for r := range results {
		done++
		fmt.Printf("job %2d done by worker %d in %v\n", r.job, r.worker, r.took.Round(time.Millisecond))
	}
	fmt.Printf("%d/10 jobs in %v with %d workers", done, time.Since(start).Round(time.Millisecond), *workers)
	if ctx.Err() != nil {
		fmt.Printf(" — stopped early: %v", ctx.Err())
	}
	fmt.Println()
}

// slowWork pretends to call a slow service (200–400 ms), but gives up as
// soon as the context is cancelled.
func slowWork(ctx context.Context) (time.Duration, error) {
	d := time.Duration(200+rand.IntN(200)) * time.Millisecond
	select {
	case <-time.After(d):
		return d, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
