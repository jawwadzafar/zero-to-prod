// dockerlab — a tiny web server to practise Dockerfiles on (chapter 9.2).
// It uses only Go's standard library, so building it downloads nothing.
//
//	docker build -t dockerlab .
//	docker run --rm -p 8080:8080 dockerlab
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}
	host, _ := os.Hostname() // inside a container: the container ID

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from %s (version %s, running as uid %d)\n", host, version, os.Getuid())
	})
	srv := &http.Server{Addr: ":8080"}

	// Stop gracefully on SIGTERM (what `docker stop` sends) or Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Println("signal received, shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("dockerlab %s listening on :8080", version)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Println("bye")
}
