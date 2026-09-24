package debugserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/debugserver"
)

func TestHandlerServesProfiles(t *testing.T) {
	ts := httptest.NewServer(debugserver.Handler())
	defer ts.Close()
	for path, want := range map[string]string{
		"/debug/pprof/":                  "goroutine",
		"/debug/pprof/goroutine?debug=1": "goroutine profile:",
	} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(b), want) {
			t.Fatalf("GET %s = %d, body missing %q", path, res.StatusCode, want)
		}
	}
}
