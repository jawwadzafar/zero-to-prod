package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/postgres"
)

// openTestDB connects to SNIP_TEST_DATABASE_URL, or skips the test when it
// isn't set (so `go test ./...` works on a laptop without Postgres).
func openTestDB(t *testing.T) *postgres.Store {
	t.Helper()
	url := os.Getenv("SNIP_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SNIP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestMigrateIsIdempotent(t *testing.T) {
	st := openTestDB(t)
	applied, err := st.Migrate(context.Background())
	if err != nil || len(applied) != 0 {
		t.Fatalf("second Migrate applied %v, err %v; want nothing", applied, err)
	}
}

func TestLinksRoundTrip(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	key, owner, err := auth.CreateKey(ctx, st, "pg-test")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := auth.Authenticate(ctx, st, key); err != nil || got.ID != owner.ID {
		t.Fatalf("Authenticate = %+v, %v", got, err)
	}

	svc := links.NewService(st, nil, nil)
	l, err := svc.Shorten(ctx, owner.ID, "https://www.postgresql.org/docs/", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, links.Link{Slug: l.Slug, URL: "https://x.example", OwnerID: owner.ID, CreatedAt: time.Now()}); !errors.Is(err, links.ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v, want ErrSlugTaken", err)
	}
	res, err := svc.Resolve(ctx, l.Slug)
	if err != nil || res.URL != "https://www.postgresql.org/docs/" {
		t.Fatalf("Resolve = %+v, %v", res, err)
	}
	list, err := svc.List(ctx, owner.ID, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}
	if err := svc.Delete(ctx, owner.ID, l.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(ctx, l.Slug); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

// Many concurrent increments must not lose any updates (chapter 6.5).
func TestAddClicksIsAtomic(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	_, owner, _ := auth.CreateKey(ctx, st, "clicks-test")
	l, err := links.NewService(st, nil, nil).Shorten(ctx, owner.ID, "https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := st.AddClicks(ctx, l.Slug, 2); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := st.Get(ctx, l.Slug)
	if got.Clicks != 100 {
		t.Fatalf("clicks = %d, want 100", got.Clicks)
	}
}

func TestRevokedKeyStopsWorking(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	key, owner, err := auth.CreateKey(ctx, st, "to-revoke")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeKey(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, st, key); !errors.Is(err, auth.ErrUnknownKey) {
		t.Fatalf("revoked key: Authenticate error = %v, want ErrUnknownKey", err)
	}
	if err := st.RevokeKey(ctx, owner.ID); !errors.Is(err, auth.ErrUnknownKey) {
		t.Fatalf("revoking twice = %v, want ErrUnknownKey", err)
	}
}
