package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/httpapi"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/metrics"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/ratelimit"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/memory"
)

// newTestServer starts snip on a random local port with in-memory storage
// and returns its URL plus a valid API key.
func newTestServer(t *testing.T, rateLimit int) (string, string) {
	t.Helper()
	st := memory.New()
	key, _, err := auth.CreateKey(context.Background(), st, "tester")
	if err != nil {
		t.Fatal(err)
	}
	api := httpapi.New(httpapi.Options{
		Links:   links.NewService(st, nil, nil),
		Keys:    st,
		Limiter: ratelimit.NewMemory(rateLimit, time.Minute),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics: metrics.New(),
		BaseURL: "http://snip.test",
	})
	ts := httptest.NewServer(api.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, key
}

func do(t *testing.T, method, url, key, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func decode(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCreateFollowAndCount(t *testing.T) {
	base, key := newTestServer(t, 100)

	res := do(t, "POST", base+"/api/links", key, `{"url":"https://go.dev/doc","slug":"godoc"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", res.StatusCode)
	}
	body := decode(t, res)
	if body["short_url"] != "http://snip.test/godoc" {
		t.Fatalf("short_url = %v", body["short_url"])
	}

	res = do(t, "GET", base+"/godoc", "", "")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://go.dev/doc" {
		t.Fatalf("redirect = %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	res = do(t, "GET", base+"/api/links/godoc", key, "")
	if got := decode(t, res)["clicks"]; got != float64(1) {
		t.Fatalf("clicks = %v, want 1", got)
	}
}

func TestErrors(t *testing.T) {
	base, key := newTestServer(t, 100)
	tests := []struct {
		name, method, path, key, body string
		wantStatus                    int
		wantCode                      string
	}{
		{"no key", "POST", "/api/links", "", `{"url":"https://go.dev"}`, 401, "unauthorized"},
		{"bad key", "POST", "/api/links", "snip_wrong", `{"url":"https://go.dev"}`, 401, "unauthorized"},
		{"bad json", "POST", "/api/links", key, `{"url":`, 400, "invalid_json"},
		{"unknown field", "POST", "/api/links", key, `{"link":"https://go.dev"}`, 400, "invalid_json"},
		{"bad url", "POST", "/api/links", key, `{"url":"javascript:alert(1)"}`, 400, "invalid_url"},
		{"reserved slug", "POST", "/api/links", key, `{"url":"https://go.dev","slug":"api"}`, 400, "invalid_slug"},
		{"missing link", "GET", "/nope123", "", "", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := do(t, tt.method, base+tt.path, tt.key, tt.body)
			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			errObj, _ := decode(t, res)["error"].(map[string]any)
			if errObj["code"] != tt.wantCode {
				t.Fatalf("error code = %v, want %s", errObj["code"], tt.wantCode)
			}
		})
	}
}

func TestDuplicateSlugConflict(t *testing.T) {
	base, key := newTestServer(t, 100)
	do(t, "POST", base+"/api/links", key, `{"url":"https://a.example","slug":"same"}`)
	res := do(t, "POST", base+"/api/links", key, `{"url":"https://b.example","slug":"same"}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
}

func TestRateLimit(t *testing.T) {
	base, key := newTestServer(t, 2)
	for i := 0; i < 2; i++ {
		if res := do(t, "POST", base+"/api/links", key, `{"url":"https://go.dev"}`); res.StatusCode != 201 {
			t.Fatalf("request %d status = %d", i, res.StatusCode)
		}
	}
	res := do(t, "POST", base+"/api/links", key, `{"url":"https://go.dev"}`)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("third request = %d (Retry-After %q), want 429 with Retry-After", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

func TestListAndDelete(t *testing.T) {
	base, key := newTestServer(t, 100)
	do(t, "POST", base+"/api/links", key, `{"url":"https://a.example","slug":"one"}`)
	do(t, "POST", base+"/api/links", key, `{"url":"https://b.example","slug":"two"}`)

	list, _ := decode(t, do(t, "GET", base+"/api/links", key, ""))["links"].([]any)
	if len(list) != 2 {
		t.Fatalf("listed %d links, want 2", len(list))
	}
	if res := do(t, "DELETE", base+"/api/links/one", key, ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", res.StatusCode)
	}
	if res := do(t, "GET", base+"/one", "", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("follow after delete = %d, want 404", res.StatusCode)
	}
}

func TestHealthAndMetrics(t *testing.T) {
	base, _ := newTestServer(t, 100)
	if res := do(t, "GET", base+"/healthz", "", ""); res.StatusCode != 200 {
		t.Fatalf("healthz = %d", res.StatusCode)
	}
	if res := do(t, "GET", base+"/readyz", "", ""); res.StatusCode != 200 {
		t.Fatalf("readyz = %d", res.StatusCode)
	}
	res := do(t, "GET", base+"/metrics", "", "")
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), "snip_http_requests_total") {
		t.Fatal("metrics missing snip_http_requests_total")
	}
	if res.Header.Get("X-Request-ID") == "" {
		t.Fatal("responses should carry X-Request-ID")
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("responses should carry X-Content-Type-Options: nosniff")
	}
}

func TestRevokedKeyIsRejected(t *testing.T) {
	st := memory.New()
	key, owner, _ := auth.CreateKey(context.Background(), st, "tester")
	api := httpapi.New(httpapi.Options{
		Links:  links.NewService(st, nil, nil),
		Keys:   st,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ts := httptest.NewServer(api.Handler())
	defer ts.Close()
	if res := do(t, "GET", ts.URL+"/api/links", key, ""); res.StatusCode != 200 {
		t.Fatalf("before revoke: %d", res.StatusCode)
	}
	if err := st.RevokeKey(context.Background(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if res := do(t, "GET", ts.URL+"/api/links", key, ""); res.StatusCode != 401 {
		t.Fatalf("after revoke: %d, want 401", res.StatusCode)
	}
}

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

// A soft dependency (Redis) being down must not fail readiness: every copy
// would leave the load balancer at once, although redirects still work
// from Postgres. A hard dependency (Postgres) being down must fail it.
func TestReadyzHardVsDegradable(t *testing.T) {
	up := pingFunc(func(context.Context) error { return nil })
	down := pingFunc(func(context.Context) error { return errors.New("connection refused") })
	cases := []struct {
		name       string
		hard, soft httpapi.Pinger
		want       int
		degraded   bool
	}{
		{"all up", up, up, 200, false},
		{"redis down", up, down, 200, true},
		{"postgres down", down, up, 503, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := httpapi.New(httpapi.Options{
				Links:      links.NewService(memory.New(), nil, nil),
				Keys:       memory.New(),
				Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
				Ready:      map[string]httpapi.Pinger{"postgres": tc.hard},
				Degradable: map[string]httpapi.Pinger{"redis": tc.soft},
			})
			ts := httptest.NewServer(api.Handler())
			defer ts.Close()
			res := do(t, "GET", ts.URL+"/readyz", "", "")
			if res.StatusCode != tc.want {
				t.Fatalf("readyz = %d, want %d", res.StatusCode, tc.want)
			}
			if got := decode(t, res)["degraded"]; got != tc.degraded {
				t.Fatalf("degraded = %v, want %v", got, tc.degraded)
			}
		})
	}
}

func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	url, key := newTestServer(t, 100)
	req, _ := http.NewRequest(http.MethodGet, url+"/api/links", nil)
	req.Header.Set("Authorization", "bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lowercase scheme: status %d, want 200", res.StatusCode)
	}
}

func TestRequestIDIsEchoedOnlyWhenPlain(t *testing.T) {
	url, _ := newTestServer(t, 100)
	for id, keep := range map[string]bool{
		"abc-123_DEF.9":             true,
		"has space":                 false,
		"<script>alert(1)</script>": false,
		strings.Repeat("a", 65):     false,
	} {
		req, _ := http.NewRequest(http.MethodGet, url+"/healthz", nil)
		req.Header.Set("X-Request-ID", id)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("X-Request-ID"); (got == id) != keep {
			t.Errorf("X-Request-ID %q: echoed %q, keep=%v", id, got, keep)
		}
	}
}
