// Package debugserver exposes Go's built-in profiler (pprof) on a separate,
// private address (chapter 13.6). It is off unless SNIP_DEBUG_ADDR is set.
//
// Never expose it publicly: profiles reveal internals, and a CPU or trace
// profile costs real CPU while it runs. Bind it to localhost, or to a port
// only reachable with kubectl port-forward.
package debugserver

import (
	"net/http"
	"net/http/pprof"
)

// Handler serves the pprof endpoints under /debug/pprof/.
// We register them on our own mux rather than importing net/http/pprof for
// its side effect, which would add them to http.DefaultServeMux.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index) // also serves heap, goroutine, block, mutex, allocs...
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile) // CPU profile: ?seconds=N
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace) // execution trace: ?seconds=N
	return mux
}
