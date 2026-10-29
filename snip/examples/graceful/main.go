// graceful — shutting down an HTTP server without dropping requests (chapter 8.4).
//
// When an orchestrator, a deploy script or `docker stop` wants a process gone,
// it sends SIGTERM and waits a while before SIGKILL. This server uses that
// window in three steps:
//
//  1. Flip readiness: /readyz starts answering 503, so load balancers stop
//     sending new requests here.
//  2. Wait a short drain delay, so they notice (health checks run every few seconds).
//  3. Shutdown: stop accepting connections, let in-flight requests finish
//     (up to a deadline), then exit.
//
//	go run ./examples/graceful                      # listens on :9091
//	curl localhost:9091/slow &                       # a request that takes 3 s
//	kill -TERM <pid>                                 # or Ctrl-C: the slow request still completes
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":9091", "listen address")
	drain := flag.Duration("drain-delay", 5*time.Second, "how long to keep serving after /readyz turns 503")
	work := flag.Duration("work", 3*time.Second, "how long /slow takes")
	flag.Parse()

	var ready atomic.Bool
	ready.Store(true)
	srv := &http.Server{Addr: *addr, Handler: newMux(&ready, *work), ReadHeaderTimeout: 5 * time.Second}

	// The context is cancelled on SIGTERM (what orchestrators and `docker stop` send) or Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("listening on %s (pid %d)", *addr, os.Getpid())

	select {
	case err := <-errc:
		log.Fatal(err) // failed to start, e.g. "address already in use"
	case <-ctx.Done():
	}

	// 1. Tell the load balancer to stop sending new requests.
	ready.Store(false)
	log.Printf("signal received: /readyz now 503; draining for %v", *drain)
	// 2. Keep serving while it notices.
	time.Sleep(*drain)
	// 3. Stop accepting connections and wait for in-flight requests, up to 10 s.
	log.Print("shutting down: waiting for in-flight requests")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced exit, requests cut off: %v", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Print("stopped cleanly")
}

// newMux serves a slow endpoint (to have something in flight) and a readiness check.
func newMux(ready *atomic.Bool, work time.Duration) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(work)
		fmt.Fprintln(w, "done")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})
	return mux
}
