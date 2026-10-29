package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"
)

// --- Middleware: code that wraps every request. ------------------------------

// Middleware takes a handler and returns a new handler that does something
// before and/or after calling it.
type Middleware func(http.Handler) http.Handler

// Chain wraps h so the FIRST middleware listed is the OUTERMOST: a request
// passes through them in the order written, and the response comes back out
// in reverse.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// ctxKey is a private type, so no other package can collide with our keys.
type ctxKey struct{}

// WithRequestID gives every request an ID, puts it in the request's context
// (so any layer can log it) and in the response (so the caller can quote it).
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()[:12]
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestID reads the ID back out of a context. "" if there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// WithTimeout gives every request a deadline. Everything below that uses
// r.Context() — the service, the repository, a database driver — gives up
// when it passes.
func WithTimeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusRecorder remembers which status code the handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// WithLogging writes one structured log line per request, after it finishes.
func WithLogging(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r) // the rest of the chain runs here
			log.InfoContext(r.Context(), "request",
				"request_id", RequestID(r.Context()), "method", r.Method, "path", r.URL.Path,
				"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
		})
	}
}

// WithRecover turns a panic in any handler into a clean 500 instead of a
// dropped connection.
func WithRecover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					log.ErrorContext(r.Context(), "panic", "panic", p, "request_id", RequestID(r.Context()))
					writeError(w, http.StatusInternalServerError, "internal", "something went wrong on our side")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
