package links_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/memory"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "https://go.dev", want: "https://go.dev"},
		{in: "  http://example.com/a?b=c  ", want: "http://example.com/a?b=c"},
		{in: "", wantErr: true},
		{in: "go.dev", wantErr: true},                                  // no scheme
		{in: "javascript:alert(1)", wantErr: true},                     // dangerous scheme
		{in: "ftp://example.com/file", wantErr: true},                  // not http(s)
		{in: "https://", wantErr: true},                                // no host
		{in: "https://bank.example@evil.example/login", wantErr: true}, // really goes to evil.example
		{in: "https://user:pass@example.com/", wantErr: true},          // credentials in a shared link
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := links.NormalizeURL(tt.in)
			if tt.wantErr {
				if !errors.Is(err, links.ErrInvalidURL) {
					t.Fatalf("NormalizeURL(%q) error = %v, want ErrInvalidURL", tt.in, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("NormalizeURL(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestValidateSlug(t *testing.T) {
	for _, ok := range []string{"abc", "my-launch_2026", "A1b2C3"} {
		if err := links.ValidateSlug(ok); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"ab", "has space", "emoji🙂", "api", "Healthz", "x/y"} {
		if err := links.ValidateSlug(bad); !errors.Is(err, links.ErrInvalidSlug) {
			t.Errorf("ValidateSlug(%q) = %v, want ErrInvalidSlug", bad, err)
		}
	}
}

func TestNewSlugIsRandomAndReadable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		s := links.NewSlug()
		if len(s) != 7 || links.ValidateSlug(s) != nil {
			t.Fatalf("bad slug %q", s)
		}
		if seen[s] {
			t.Fatalf("duplicate slug %q in 1000 tries", s)
		}
		seen[s] = true
	}
}

// fakeCache counts calls so we can check the cache is used.
type fakeCache struct {
	data       map[string]string
	gets, sets int
	down       bool // simulates a Redis outage: every call fails
}

var errCacheDown = errors.New("dial tcp: connection refused")

func (c *fakeCache) GetURL(_ context.Context, slug string) (string, bool, error) {
	c.gets++
	if c.down {
		return "", false, errCacheDown
	}
	u, ok := c.data[slug]
	return u, ok, nil
}
func (c *fakeCache) SetURL(_ context.Context, slug, url string) error {
	c.sets++
	if c.down {
		return errCacheDown
	}
	c.data[slug] = url
	return nil
}
func (c *fakeCache) Delete(_ context.Context, slug string) error { delete(c.data, slug); return nil }

func TestServiceShortenResolveDelete(t *testing.T) {
	ctx := context.Background()
	cache := &fakeCache{data: map[string]string{}}
	svc := links.NewService(memory.New(), cache, nil)

	l, err := svc.Shorten(ctx, 1, "https://go.dev/doc", "")
	if err != nil {
		t.Fatal(err)
	}

	// First resolve misses the cache and fills it; the second hits it.
	r1, err := svc.Resolve(ctx, l.Slug)
	if err != nil || r1.URL != "https://go.dev/doc" || r1.CacheHit {
		t.Fatalf("first resolve = %+v, %v", r1, err)
	}
	r2, _ := svc.Resolve(ctx, l.Slug)
	if !r2.CacheHit {
		t.Fatal("second resolve should hit the cache")
	}

	// Custom slugs must be unique.
	if _, err := svc.Shorten(ctx, 1, "https://example.com", "launch"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Shorten(ctx, 2, "https://example.org", "launch"); !errors.Is(err, links.ErrSlugTaken) {
		t.Fatalf("duplicate slug error = %v, want ErrSlugTaken", err)
	}

	// Only the owner can see or delete a link.
	if _, err := svc.Get(ctx, 2, l.Slug); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("other owner Get = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, 2, l.Slug); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("other owner Delete = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, 1, l.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, l.Slug); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("resolve after delete = %v, want ErrNotFound (cache must be evicted)", err)
	}
}

// With the cache down, redirects still work from the store, and snip
// doesn't wait on the broken cache a second time to fill it (chapter 13.5).
func TestResolveWithCacheDown(t *testing.T) {
	ctx := context.Background()
	cache := &fakeCache{data: map[string]string{}}
	svc := links.NewService(memory.New(), cache, nil)
	l, err := svc.Shorten(ctx, 1, "https://go.dev/doc", "")
	if err != nil {
		t.Fatal(err)
	}
	cache.down = true
	r, err := svc.Resolve(ctx, l.Slug)
	if err != nil || r.URL != "https://go.dev/doc" || r.CacheHit {
		t.Fatalf("resolve with cache down = %+v, %v", r, err)
	}
	if cache.gets != 1 || cache.sets != 0 {
		t.Fatalf("cache gets=%d sets=%d, want 1 and 0", cache.gets, cache.sets)
	}
}

func TestRecordClickWithoutQueueCountsDirectly(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	svc := links.NewService(st, nil, nil)
	l, err := svc.Shorten(ctx, 1, "https://go.dev", "golang")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := svc.RecordClick(ctx, l.Slug); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := svc.Get(ctx, 1, "golang")
	if got.Clicks != 3 {
		t.Fatalf("clicks = %d, want 3", got.Clicks)
	}
}

// Benchmarks (chapter 8.5): go test -bench . -benchmem ./internal/links
func BenchmarkNewSlug(b *testing.B) {
	for b.Loop() {
		links.NewSlug()
	}
}

func BenchmarkValidateSlug(b *testing.B) {
	for b.Loop() {
		links.ValidateSlug("my-launch-2026")
	}
}
