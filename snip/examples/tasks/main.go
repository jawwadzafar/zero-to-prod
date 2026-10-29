// tasks — a small to-do API in four layers (chapters 5.3 and 5.4):
//
//	handler.go     the handler ("controller"): HTTP in, HTTP out
//	service.go     the service: business rules
//	repository.go  the repository: storage behind an interface
//	middleware.go  middleware: request IDs, deadlines, logging, panic recovery
//	main.go        wiring: build each layer and hand it to the next
//
// Run it and talk to it with curl:
//
//	go run ./examples/tasks
//	curl -i -X POST localhost:8081/tasks -d '{"title":"buy milk"}'
//	curl -i localhost:8081/tasks/1
//	go run ./examples/tasks -delay 3s      # a "slow database": requests hit the 2s deadline
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8081", "where to listen")
	delay := flag.Duration("delay", 0, "pretend every storage call takes this long")
	timeout := flag.Duration("timeout", 2*time.Second, "deadline for each request")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Wiring: the only place that knows every concrete type.
	repo := NewMemoryRepo()
	repo.Delay = *delay
	svc := NewService(repo)
	h := &Handler{svc: svc, log: log}

	// The request passes through the middleware in this order, then the router.
	handler := Chain(h.Routes(),
		WithRequestID,
		WithLogging(log),
		WithRecover(log),
		WithTimeout(*timeout),
	)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown: on Ctrl-C or SIGTERM, finish in-flight requests first.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Info("tasks API listening", "addr", *addr, "delay", *delay, "timeout", *timeout)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("stopped cleanly")
}
