package links_test

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

// Fuzz tests (chapter 7.13). `go test ./...` runs only the seed inputs below,
// as ordinary tests. To let Go invent millions of new inputs, run one target:
//
//	go test ./internal/links -run='^$' -fuzz=FuzzNormalizeURL -fuzztime=30s
//
// A fuzz test can't know the right answer for a random input, so it checks
// properties that must hold for every input.

func FuzzNormalizeURL(f *testing.F) {
	for _, seed := range []string{
		"https://go.dev",
		"  http://example.com/a?b=c  ",
		"",
		"go.dev",
		"javascript:alert(1)",
		"https://",
		"https://bank.example@evil.example/login",
		"http://[::1]:8080/path#frag",
		"https://example.com/café",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got, err := links.NormalizeURL(raw)
		if err != nil {
			return // refusing is always allowed
		}
		// Property 1: what we accept is an absolute http(s) URL with a host
		// and no username or password, the promises NormalizeURL makes.
		u, perr := url.Parse(got)
		if perr != nil {
			t.Fatalf("NormalizeURL(%q) = %q, which does not parse: %v", raw, got, perr)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			t.Fatalf("NormalizeURL(%q) = %q: scheme %q, host %q, user %v", raw, got, u.Scheme, u.Host, u.User)
		}
		// Property 2: the result fits the database column
		// (CHECK (length(url) <= 2048) in migrations/0001_init.sql).
		if len(got) > 2048 {
			t.Fatalf("NormalizeURL(%d-byte input) returned %d bytes, more than the 2048 the database accepts", len(raw), len(got))
		}
		// Property 3: normalising twice changes nothing.
		again, err := links.NormalizeURL(got)
		if err != nil || again != got {
			t.Fatalf("not idempotent: NormalizeURL(%q) = %q, then %q, %v", raw, got, again, err)
		}
	})
}

var slugChars = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func FuzzValidateSlug(f *testing.F) {
	for _, seed := range []string{"abc", "my-launch_2026", "ab", "api", "App", "Healthz", "x/y", "emoji🙂", "../etc"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if links.ValidateSlug(s) != nil {
			return
		}
		// An accepted slug becomes one path segment (GET /{slug}) and a Redis
		// key: it must be 3-32 plain characters and never one of snip's routes.
		if len(s) < 3 || len(s) > 32 || !slugChars.MatchString(s) {
			t.Fatalf("ValidateSlug accepted %q", s)
		}
		switch strings.ToLower(s) {
		case "api", "app", "healthz", "readyz", "metrics":
			t.Fatalf("ValidateSlug accepted reserved slug %q", s)
		}
	})
}
