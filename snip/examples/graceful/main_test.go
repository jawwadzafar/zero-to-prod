package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadyzFlips(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)
	mux := newMux(&ready, 0)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ready: got %d, want 200", rec.Code)
	}

	ready.Store(false)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("draining: got %d, want 503", rec.Code)
	}
}

// An in-flight request must complete even though Shutdown starts while it runs.
func TestShutdownFinishesInFlight(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: newMux(&ready, 300*time.Millisecond)}
	go srv.Serve(ln)

	result := make(chan int, 1)
	go func() {
		res, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			result <- 0
			return
		}
		res.Body.Close()
		result <- res.StatusCode
	}()

	time.Sleep(100 * time.Millisecond) // the request is now in flight
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if code := <-result; code != http.StatusOK {
		t.Fatalf("in-flight request: got %d, want 200", code)
	}
}
