// Package metrics defines the numbers snip exposes on /metrics for
// Prometheus to scrape (chapter 13.2).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics is the set of counters and histograms snip updates.
type Metrics struct {
	Registry        *prometheus.Registry
	Requests        *prometheus.CounterVec   // by route, method, status code
	RequestDuration *prometheus.HistogramVec // seconds, by route
	Redirects       *prometheus.CounterVec   // by cache result: hit | miss
	ClickErrors     prometheus.Counter       // clicks we failed to record
	ClicksProcessed prometheus.Counter       // events handled by the worker
}

// New registers snip's metrics plus Go runtime and process metrics.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "snip_http_requests_total", Help: "HTTP requests handled.",
		}, []string{"route", "method", "code"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "snip_http_request_duration_seconds",
			Help:    "Time to handle HTTP requests.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		}, []string{"route"}),
		Redirects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "snip_redirects_total", Help: "Redirects served, by cache result.",
		}, []string{"cache"}),
		ClickErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "snip_click_record_errors_total", Help: "Clicks that could not be recorded.",
		}),
		ClicksProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "snip_worker_clicks_processed_total", Help: "Click events processed by the worker.",
		}),
	}
	reg.MustRegister(m.Requests, m.RequestDuration, m.Redirects, m.ClickErrors, m.ClicksProcessed,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}
