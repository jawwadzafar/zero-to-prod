// realtime — the same stream of events served four ways (chapter 8.7):
// short polling, long polling, Server-Sent Events and a WebSocket.
//
//	go run ./examples/realtime                     # http://localhost:8090
//	curl -N localhost:8090/events                  # watch the SSE stream
//	curl -X POST localhost:8090/publish -d hello   # publish an event
//	curl 'localhost:8090/poll?after=0'             # short poll
//	curl 'localhost:8090/longpoll?after=999999'    # long poll: waits for the next event
//
// With -redis, several copies share events through Redis pub/sub, so a
// client connected to one copy sees events published on another:
//
//	go run ./examples/realtime -redis localhost:6379 -addr :8090
//	go run ./examples/realtime -redis localhost:6379 -addr :8091 -tick 0
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
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8090", "address to listen on")
	redisAddr := flag.String("redis", "", "Redis host:port; empty = single server, no fan-out")
	tick := flag.Duration("tick", 3*time.Second, "publish a demo event this often (0 = off)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub := NewHub()
	s := newServer(hub)
	if *redisAddr != "" {
		bus, err := newRedisBus(ctx, *redisAddr, hub)
		if err != nil {
			log.Fatal(err)
		}
		defer bus.Close()
		s.publish = bus.Publish // publish to Redis; every copy (this one too) receives it
		log.Printf("fan-out through Redis at %s", *redisAddr)
	}

	if *tick > 0 {
		go func() {
			t := time.NewTicker(*tick)
			defer t.Stop()
			for n := 1; ; n++ {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if _, err := s.publish(ctx, fmt.Sprintf("tick %d (clients on this copy: %d)", n, hub.Subscribers())); err != nil {
						log.Printf("publish: %v", err)
					}
				}
			}
		}()
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second, // normal responses; the stream handlers lift it for themselves
	}
	// Shutdown waits for connections to go idle, and a stream never does.
	// Closing the hub ends every stream, so shutdown can finish.
	srv.RegisterOnShutdown(hub.Close)

	go func() {
		log.Printf("listening on http://localhost%s", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	log.Print("shutting down: closing streams")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
