// Package httpapi is snip's HTTP interface: routes, handlers and middleware.
// It translates HTTP into calls on links.Service and back.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/metrics"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/ratelimit"
)

// Pinger is anything /readyz should check (the database, Redis).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server holds everything the handlers need.
type Server struct {
	links   *links.Service
	keys    auth.KeyStore
	limiter ratelimit.Limiter
	log     *slog.Logger
	metrics *metrics.Metrics
	baseURL string
	ready   map[string]Pinger
}

// Options configures a Server. Limiter, Metrics and Ready are optional.
type Options struct {
	Links   *links.Service
	Keys    auth.KeyStore
	Limiter ratelimit.Limiter
	Logger  *slog.Logger
	Metrics *metrics.Metrics
	BaseURL string
	Ready   map[string]Pinger
}

// New builds a Server.
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Server{links: o.Links, keys: o.Keys, limiter: o.Limiter, log: o.Logger,
		metrics: o.Metrics, baseURL: strings.TrimRight(o.BaseURL, "/"), ready: o.Ready}
}

// Handler returns the full HTTP handler with every route and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Go 1.22+ patterns: "METHOD /path/{wildcard}".
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /{slug}", s.redirect)
	mux.HandleFunc("POST /api/links", s.requireKey(s.createLink))
	mux.HandleFunc("GET /api/links", s.requireKey(s.listLinks))
	mux.HandleFunc("GET /api/links/{slug}", s.requireKey(s.getLink))
	mux.HandleFunc("DELETE /api/links/{slug}", s.requireKey(s.deleteLink))
	if s.metrics != nil {
		mux.Handle("GET /metrics", promhttp.HandlerFor(s.metrics.Registry, promhttp.HandlerOpts{}))
	}
	return s.observe(mux)
}

func (s *Server) home(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "snip",
		"docs":    "https://jawwadzafar.github.io/zero-to-prod/",
	})
}

// healthz answers "is the process alive?" — it never checks dependencies, so
// a database blip doesn't make an orchestrator kill healthy processes.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz answers "can this process serve traffic right now?" by pinging each
// dependency with a short timeout.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	status := map[string]string{}
	healthy := true
	for name, p := range s.ready {
		if err := p.Ping(ctx); err != nil {
			status[name] = "unavailable"
			healthy = false
		} else {
			status[name] = "ok"
		}
	}
	code := http.StatusOK
	if !healthy {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{"ready": healthy, "checks": status})
}

// redirect is the hot path: look the slug up and send the browser on its way.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	res, err := s.links.Resolve(r.Context(), slug)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	if s.metrics != nil {
		result := "miss"
		if res.CacheHit {
			result = "hit"
		}
		s.metrics.Redirects.WithLabelValues(result).Inc()
	}
	if err := s.links.RecordClick(r.Context(), slug); err != nil {
		// Never fail a redirect because counting failed.
		s.log.WarnContext(r.Context(), "click not recorded", "slug", slug, "err", err)
		if s.metrics != nil {
			s.metrics.ClickErrors.Inc()
		}
	}
	// 302 (temporary), not 301 (permanent): browsers cache 301s forever,
	// so later clicks would skip snip entirely and never be counted.
	w.Header().Set("Cache-Control", "private, max-age=0")
	http.Redirect(w, r, res.URL, http.StatusFound)
}

type createRequest struct {
	URL  string `json:"url"`
	Slug string `json:"slug,omitempty"`
}

type linkResponse struct {
	links.Link
	ShortURL string `json:"short_url"`
}

func (s *Server) toResponse(l links.Link) linkResponse {
	return linkResponse{Link: l, ShortURL: s.baseURL + "/" + l.Slug}
}

const maxBodyBytes = 1 << 14 // 16 KiB is plenty for {"url": …}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	owner := ownerFrom(r.Context())
	if s.limiter != nil {
		allowed, retry, err := s.limiter.Allow(r.Context(), "create:"+strconv.FormatInt(owner.ID, 10))
		if err != nil {
			s.log.WarnContext(r.Context(), "rate limiter unavailable", "err", err)
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many links created; slow down")
			return
		}
	}
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "body must be JSON like {\"url\": \"https://…\"}")
		return
	}
	l, err := s.links.Shorten(r.Context(), owner.ID, req.URL, req.Slug)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/links/"+l.Slug)
	writeJSON(w, http.StatusCreated, s.toResponse(l))
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ls, err := s.links.List(r.Context(), ownerFrom(r.Context()).ID, limit)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	out := make([]linkResponse, 0, len(ls))
	for _, l := range ls {
		out = append(out, s.toResponse(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": out})
}

func (s *Server) getLink(w http.ResponseWriter, r *http.Request) {
	l, err := s.links.Get(r.Context(), ownerFrom(r.Context()).ID, r.PathValue("slug"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toResponse(l))
}

func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	if err := s.links.Delete(r.Context(), ownerFrom(r.Context()).ID, r.PathValue("slug")); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
