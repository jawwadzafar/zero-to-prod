// Command mtls runs the chapter 7.11 demo: one server that only the worker
// may call, and four callers trying.
//
//	go run ./examples/mtls/cmd
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/mtls"
)

func must[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return v
}

func main() {
	log.SetFlags(0)
	ca := must(mtls.NewCA("snip internal CA"))
	server := httptest.NewUnstartedServer(mtls.Allow([]string{"spiffe://snip.local/worker"},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := mtls.PeerID(r)
			fmt.Fprintf(w, "200 hello %s", id)
		})))
	server.TLS = mtls.ServerTLS(ca, must(ca.Issue("spiffe://snip.local/api", []string{"localhost"}, time.Hour)))
	server.Config.ErrorLog = log.New(io.Discard, "", 0) // the handshake errors are printed below instead
	server.StartTLS()
	defer server.Close()

	attacker := must(mtls.NewCA("some other CA"))
	callers := []struct {
		name string
		cert *tls.Certificate
	}{
		{"worker, certificate from our CA", ptr(must(ca.Issue("spiffe://snip.local/worker", nil, time.Hour)))},
		{"reporting job, certificate from our CA", ptr(must(ca.Issue("spiffe://snip.local/reporting", nil, time.Hour)))},
		{"no certificate", nil},
		{"'worker' signed by another CA", ptr(must(attacker.Issue("spiffe://snip.local/worker", nil, time.Hour)))},
		{"worker, expired certificate", ptr(must(ca.Issue("spiffe://snip.local/worker", nil, -2*time.Minute)))},
	}
	for _, c := range callers {
		cfg := mtls.ClientTLS(ca, tls.Certificate{})
		cfg.Certificates = nil
		if c.cert != nil {
			cfg.Certificates = []tls.Certificate{*c.cert}
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
		res, err := client.Get(server.URL)
		if err != nil {
			fmt.Printf("%-40s refused during the TLS handshake\n", c.name)
			continue
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			fmt.Printf("%-40s connected, then %d: %s", c.name, res.StatusCode, body)
			continue
		}
		fmt.Printf("%-40s %s\n", c.name, body)
	}
}

func ptr[T any](v T) *T { return &v }
