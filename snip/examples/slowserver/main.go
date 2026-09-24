// slowserver — a server with a known, fixed capacity (chapter 8.5).
//
// Every request takes 10 ms of "work", and only 4 can be worked on at once
// (like 4 database connections). So its capacity is exactly
// 4 ÷ 0.010 s = 400 requests per second. Load-test it and watch what happens
// as you approach — and pass — that number.
//
//	go run ./examples/slowserver            # listens on :9090
//	go run ./examples/loadgen -url http://localhost:9090/ -rate 300 -d 5s
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	slots = 4                     // how many requests can be worked on at once
	work  = 10 * time.Millisecond // how long each one takes
)

func main() {
	sem := make(chan struct{}, slots) // a semaphore: a channel with 4 spaces
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sem <- struct{}{}        // wait for a free slot (this wait is the queue)
		defer func() { <-sem }() // give the slot back when done
		time.Sleep(work)
		fmt.Fprintln(w, "ok")
	})
	log.Printf("slowserver on :9090 — capacity %d req/s", int(slots*time.Second/work))
	log.Fatal(http.ListenAndServe(":9090", nil))
}
