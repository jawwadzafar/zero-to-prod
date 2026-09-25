package mtls

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type setup struct {
	ca  *CA
	srv *httptest.Server
}

func newSetup(t *testing.T) setup {
	t.Helper()
	ca, err := NewCA("snip internal CA")
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := ca.Issue("spiffe://snip.local/api", []string{"localhost"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := Allow([]string{"spiffe://snip.local/worker"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := PeerID(r)
		_, _ = io.WriteString(w, "hello "+id)
	}))
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = ServerTLS(ca, serverCert)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return setup{ca: ca, srv: srv}
}

func (s setup) call(t *testing.T, cert *tlsCert, roots *CA) (int, string, error) {
	t.Helper()
	tr := &http.Transport{TLSClientConfig: ClientTLS(roots, cert.c)}
	if !cert.present {
		tr.TLSClientConfig.Certificates = nil
	}
	res, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get(s.srv.URL)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), nil
}

type tlsCert struct {
	c       tls.Certificate
	present bool
}

func TestMutualTLS(t *testing.T) {
	s := newSetup(t)
	issue := func(ca *CA, id string, life time.Duration) *tlsCert {
		c, err := ca.Issue(id, nil, life)
		if err != nil {
			t.Fatal(err)
		}
		return &tlsCert{c: c, present: true}
	}

	// The worker, with a certificate from our CA: allowed.
	code, body, err := s.call(t, issue(s.ca, "spiffe://snip.local/worker", time.Hour), s.ca)
	if err != nil || code != 200 || body != "hello spiffe://snip.local/worker" {
		t.Fatalf("worker: %d %q %v", code, body, err)
	}

	// A valid certificate, but an identity that isn't on the allow list: authenticated, not authorized.
	code, _, err = s.call(t, issue(s.ca, "spiffe://snip.local/reporting", time.Hour), s.ca)
	if err != nil || code != 403 {
		t.Fatalf("reporting: %d %v, want 403", code, err)
	}

	// No client certificate at all: the TLS handshake itself fails.
	if _, _, err := s.call(t, &tlsCert{present: false}, s.ca); err == nil {
		t.Fatal("a caller without a certificate must not connect")
	}

	// The right name, but signed by someone else's CA: rejected.
	other, _ := NewCA("attacker CA")
	forged := issue(other, "spiffe://snip.local/worker", time.Hour)
	if _, _, err := s.call(t, forged, s.ca); err == nil {
		t.Fatal("a certificate from another CA must be rejected")
	}

	// Our CA, but expired.
	expired := issue(s.ca, "spiffe://snip.local/worker", -2*time.Minute)
	if _, _, err := s.call(t, expired, s.ca); err == nil {
		t.Fatal("an expired certificate must be rejected")
	}
}
