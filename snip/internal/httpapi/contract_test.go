package httpapi_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestErrorCodesAreInContract fails if a handler can send an error code that
// the OpenAPI contract (api/openapi.yaml) doesn't list — so clients reading
// the contract are never surprised (chapter 5.5).
func TestErrorCodesAreInContract(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	enum := regexp.MustCompile(`enum: \[(invalid_json[^\]]*)\]`).FindSubmatch(spec)
	if enum == nil {
		t.Fatal("could not find the error code enum in api/openapi.yaml")
	}
	documented := map[string]bool{}
	for _, c := range strings.Split(string(enum[1]), ",") {
		documented[strings.TrimSpace(c)] = true
	}

	sources, _ := filepath.Glob("*.go")
	used := regexp.MustCompile(`writeError\([^,]+,\s*[^,]+,\s*"([a-z_]+)"`)
	found := 0
	for _, f := range sources {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range used.FindAllSubmatch(src, -1) {
			found++
			if code := string(m[1]); !documented[code] {
				t.Errorf("%s sends error code %q, which api/openapi.yaml does not document", f, code)
			}
		}
	}
	if found == 0 {
		t.Fatal("found no writeError calls — has the code changed shape?")
	}
}
