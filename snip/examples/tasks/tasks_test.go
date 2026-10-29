package main

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
)

// The service is tested on its own, with the in-memory repository: no HTTP.
func TestServiceRejectsBadTitles(t *testing.T) {
	svc := NewService(NewMemoryRepo())
	for _, title := range []string{"", "   ", strings.Repeat("x", 201)} {
		if _, err := svc.Create(context.Background(), title); !errors.Is(err, ErrInvalidTitle) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidTitle", title, err)
		}
	}
}

// brokenRepo is a fake repository that always fails, to check that an
// unexpected error becomes a 500 that reveals nothing internal.
type brokenRepo struct{}

func (brokenRepo) Create(context.Context, Task) (Task, error) {
	return Task{}, errors.New("disk on fire")
}
func (brokenRepo) Get(context.Context, int64) (Task, error) {
	return Task{}, errors.New("disk on fire")
}
func (brokenRepo) List(context.Context, int) ([]Task, error) {
	return nil, errors.New("disk on fire")
}

func newTestServer(t *testing.T, repo Repository, timeout time.Duration) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &Handler{svc: NewService(repo), log: log}
	srv := httptest.NewServer(Chain(h.Routes(), WithRequestID, WithLogging(log), WithRecover(log), WithTimeout(timeout)))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPI(t *testing.T) {
	slow := NewMemoryRepo()
	slow.Delay = 200 * time.Millisecond
	tests := []struct {
		name         string
		repo         Repository
		method, path string
		body         string
		wantStatus   int
		wantCode     string // the error code, if any
	}{
		{"create", NewMemoryRepo(), "POST", "/tasks", `{"title":"buy milk"}`, 201, ""},
		{"broken json", NewMemoryRepo(), "POST", "/tasks", `{"title":`, 400, "invalid_json"},
		{"unknown field", NewMemoryRepo(), "POST", "/tasks", `{"titel":"x"}`, 400, "invalid_json"},
		{"empty title", NewMemoryRepo(), "POST", "/tasks", `{"title":"  "}`, 400, "invalid_title"},
		{"too big", NewMemoryRepo(), "POST", "/tasks", `{"title":"` + strings.Repeat("x", 5000) + `"}`, 413, "body_too_large"},
		{"bad id", NewMemoryRepo(), "GET", "/tasks/abc", "", 400, "invalid_id"},
		{"missing", NewMemoryRepo(), "GET", "/tasks/99", "", 404, "not_found"},
		{"wrong method", NewMemoryRepo(), "DELETE", "/tasks/1", "", 405, ""},
		{"storage broken", brokenRepo{}, "GET", "/tasks", "", 500, "internal"},
		{"deadline passed", slow, "GET", "/tasks", "", 503, "timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.repo, 50*time.Millisecond)
			req, _ := http.NewRequest(tt.method, srv.URL+tt.path, strings.NewReader(tt.body))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Header.Get("X-Request-ID") == "" {
				t.Error("missing X-Request-ID header")
			}
			if tt.wantCode == "" {
				return
			}
			var body struct {
				Error struct{ Code, Message string }
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error body is not JSON: %v", err)
			}
			if body.Error.Code != tt.wantCode {
				t.Fatalf("error code = %q, want %q", body.Error.Code, tt.wantCode)
			}
			if strings.Contains(body.Error.Message, "disk on fire") {
				t.Fatal("internal error details leaked to the client")
			}
		})
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+" before")
				next.ServeHTTP(w, r)
				order = append(order, name+" after")
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mark("A"), mark("B"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	want := "A before, B before, handler, B after, A after"
	if got := strings.Join(order, ", "); got != want {
		t.Fatalf("order = %s\nwant    %s", got, want)
	}
}
