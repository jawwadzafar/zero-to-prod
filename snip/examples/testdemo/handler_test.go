package testdemo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// httptest.NewRecorder is a fake ResponseWriter: call the handler directly,
// then inspect what it wrote. No network, no port.
func TestHello(t *testing.T) {
	tests := []struct {
		name, url  string
		wantStatus int
		wantBody   string
	}{
		{"default name", "/hello", 200, "hello, world"},
		{"given name", "/hello?name=Ada", 200, "hello, Ada"},
		{"name too long", "/hello?name=" + strings.Repeat("a", 51), 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			Hello(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody == "" {
				return
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if body["greeting"] != tt.wantBody {
				t.Fatalf("greeting = %q, want %q", body["greeting"], tt.wantBody)
			}
		})
	}
}
