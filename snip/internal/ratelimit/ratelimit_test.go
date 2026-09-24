package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryFixedWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 10, 0, time.UTC)
	l := NewMemory(3, time.Minute)
	l.now = func() time.Time { return now }
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if ok, _, _ := l.Allow(ctx, "k"); !ok {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	ok, retry, _ := l.Allow(ctx, "k")
	if ok || retry != 50*time.Second {
		t.Fatalf("4th request: allowed=%v retry=%v, want blocked with 50s", ok, retry)
	}
	if ok, _, _ := l.Allow(ctx, "other"); !ok {
		t.Fatal("a different key has its own budget")
	}
	now = now.Add(time.Minute) // next window
	if ok, _, _ := l.Allow(ctx, "k"); !ok {
		t.Fatal("a new window resets the count")
	}
}
