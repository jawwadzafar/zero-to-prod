// A deliberate race condition (chapters 1.2, 4.4 and 4.5).
//
//	go run ./examples/race          # the count is usually wrong
//	go run -race ./examples/race    # the race detector explains why
//	go run ./examples/race -safe    # fixed with a mutex: always 1000
package main

import (
	"flag"
	"fmt"
	"sync"
)

func main() {
	safe := flag.Bool("safe", false, "protect the counter with a mutex")
	flag.Parse()

	clicks := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ { // 1000 "visitors" clicking at the same time
		wg.Add(1)
		go func() {
			defer wg.Done()
			if *safe {
				mu.Lock()
				clicks++ // read, add, write — with only one goroutine at a time
				mu.Unlock()
				return
			}
			clicks++ // read, add, write — goroutines can interleave and lose updates
		}()
	}
	wg.Wait()
	fmt.Println("clicks counted:", clicks, "(expected 1000)")
}
