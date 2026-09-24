package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
)

type ctxKey int

const (
	ownerKey ctxKey = iota
	requestIDKey
)

// statusRecorder remembers the status code a handler wrote, for logs and metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// observe wraps every request: gives it an ID, recovers from panics, and
// records a log line and metrics when it finishes.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		// Browsers must treat responses as the declared type, never "sniff"
		// JSON into something executable (chapter 7.5).
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		r = r.WithContext(ctx)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(ctx, "panic", "panic", p, "request_id", id)
				writeError(rec, http.StatusInternalServerError, "internal", "something went wrong on our side")
			}
			route := r.Pattern // e.g. "GET /{slug}" — low-cardinality, safe as a metric label
			if route == "" {
				route = "unmatched"
			}
			elapsed := time.Since(start)
			if s.metrics != nil {
				s.metrics.Requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
				s.metrics.RequestDuration.WithLabelValues(route).Observe(elapsed.Seconds())
			}
			if route != "GET /healthz" && route != "GET /metrics" { // keep logs about real traffic
				s.log.InfoContext(ctx, "request",
					"request_id", id, "method", r.Method, "route", route, "path", r.URL.Path,
					"status", rec.status, "duration_ms", elapsed.Milliseconds())
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// requireKey rejects requests without a valid "Authorization: Bearer snip_…"
// header and puts the key's owner into the request context.
func (s *Server) requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="snip"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid API key")
			return
		}
		owner, err := auth.Authenticate(r.Context(), s.keys, strings.TrimSpace(key))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="snip"`)
			s.writeDomainError(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ownerKey, owner)))
	}
}

func ownerFrom(ctx context.Context) auth.Owner {
	o, _ := ctx.Value(ownerKey).(auth.Owner)
	return o
}
