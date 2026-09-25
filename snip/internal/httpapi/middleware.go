package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true // an implicit 200
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer (to flush, or
// set deadlines) through this wrapper.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// validRequestID accepts IDs from upstream proxies only if they're short and
// plain, since they're echoed into logs and response headers.
func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// observe wraps every request: gives it an ID, recovers from panics, and
// records a log line and metrics when it finishes.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
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
				if p == http.ErrAbortHandler { //nolint:errorlint // a sentinel panic value, compared as net/http does
					panic(p) // the handler asked net/http to abort the response; let it
				}
				s.log.ErrorContext(ctx, "panic", "panic", p, "request_id", id)
				if rec.wroteHeader {
					rec.status = http.StatusInternalServerError // too late to change the response; record the failure
				} else {
					writeError(rec, http.StatusInternalServerError, "internal", "something went wrong on our side")
				}
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
			attrs := []any{"request_id", id, "method", r.Method, "route", route, "path", r.URL.Path,
				"status", rec.status, "duration_ms", elapsed.Milliseconds()}
			// With tracing on (chapter 13.3), name the request's span by its route and
			// put the trace ID in the log line, so logs and traces link to each other.
			if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
				span.SetName(route)
				span.SetAttributes(attribute.String("http.route", route), attribute.String("snip.request_id", id))
				attrs = append(attrs, "trace_id", span.SpanContext().TraceID().String())
			}
			if route != "GET /healthz" && route != "GET /metrics" { // keep logs about real traffic
				s.log.InfoContext(ctx, "request", attrs...)
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
		// The scheme name is case-insensitive (RFC 9110): "bearer" works too.
		scheme, key, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
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
