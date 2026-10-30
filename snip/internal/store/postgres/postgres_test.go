package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/audit"
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

func TestRotatedKeyKeepsOwner(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	oldKey, owner, err := auth.CreateKey(ctx, st, "to-rotate")
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := auth.RotateKey(ctx, st, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, st, oldKey); !errors.Is(err, auth.ErrUnknownKey) {
		t.Fatalf("old key after rotate: Authenticate error = %v, want ErrUnknownKey", err)
	}
	got, err := auth.Authenticate(ctx, st, newKey)
	if err != nil || got.ID != owner.ID {
		t.Fatalf("new key: owner %v, err %v; want owner %d", got, err, owner.ID)
	}
	if err := st.RevokeKey(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RotateKey(ctx, st, owner.ID); !errors.Is(err, auth.ErrUnknownKey) {
		t.Fatalf("rotating a revoked key = %v, want ErrUnknownKey", err)
	}
}

// rawConn is a plain connection for statements snip itself never sends.
func rawConn(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), os.Getenv("SNIP_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func TestAuditTrailIsWritten(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	reqCtx := audit.WithRequestID(ctx, "req-audit-1")
	_, owner, err := auth.CreateKey(ctx, st, "audited")
	if err != nil {
		t.Fatal(err)
	}
	svc := links.NewService(st, nil, nil)
	l, err := svc.Shorten(reqCtx, owner.ID, "https://go.dev/doc/", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(reqCtx, owner.ID+1000, l.Slug); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("delete by another owner = %v", err)
	}
	if err := svc.Delete(reqCtx, owner.ID, l.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RotateKey(ctx, st, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeKey(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeKey(ctx, owner.ID); !errors.Is(err, auth.ErrUnknownKey) { // already revoked: no event
		t.Fatalf("revoking twice = %v", err)
	}

	got, err := st.AuditLog(ctx, owner.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		action    audit.Action
		slug, req string
		detail    map[string]string
	}{ // newest first
		{audit.KeyRevoked, "", "", nil},
		{audit.KeyRotated, "", "", nil},
		{audit.LinkDeleted, l.Slug, "req-audit-1", map[string]string{"url": "https://go.dev/doc/"}},
		{audit.LinkCreated, l.Slug, "req-audit-1", map[string]string{"url": "https://go.dev/doc/"}},
		{audit.KeyCreated, "", "", map[string]string{"name": "audited"}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		e := got[i]
		if e.Action != w.action || e.Slug != w.slug || e.RequestID != w.req || e.OwnerID != owner.ID || e.At.IsZero() ||
			len(e.Detail) != len(w.detail) || e.Detail["url"] != w.detail["url"] || e.Detail["name"] != w.detail["name"] {
			t.Errorf("event %d = %+v, want %+v", i, e, w)
		}
	}
}

// If the audit row can't be written, the delete doesn't happen either: they
// share one transaction. To make the insert fail, the test briefly adds a
// constraint that refuses "link.deleted" rows.
func TestDeleteWithoutAuditRowRollsBack(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	conn := rawConn(t)
	_, owner, _ := auth.CreateKey(ctx, st, "rollback")
	l, err := links.NewService(st, nil, nil).Shorten(ctx, owner.ID, "https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_no_deletes
		CHECK (action <> 'link.deleted') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS test_no_deletes`)
	})
	if err := st.Delete(ctx, l.Slug, owner.ID); err == nil {
		t.Fatal("Delete succeeded although its audit row could not be written")
	}
	if _, err := st.Get(ctx, l.Slug); err != nil {
		t.Fatalf("link gone after a failed audit write: %v", err)
	}
}

// Nobody can rewrite the trail, not even with snip's own credentials: the
// trigger in 0002_audit_events.sql refuses UPDATE, DELETE and TRUNCATE.
func TestAuditTrailIsAppendOnly(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()
	conn := rawConn(t)
	_, owner, _ := auth.CreateKey(ctx, st, "tamper")
	for _, sql := range []string{
		`UPDATE audit_events SET action = 'key.rotated' WHERE owner_id = $1`,
		`DELETE FROM audit_events WHERE owner_id = $1`,
	} {
		_, err := conn.Exec(ctx, sql, owner.ID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" || !strings.Contains(pgErr.Message, "append-only") {
			t.Errorf("%s: err = %v, want the append-only exception (42501)", sql, err)
		}
	}
	if _, err := conn.Exec(ctx, `TRUNCATE audit_events`); err == nil {
		t.Error("TRUNCATE audit_events succeeded")
	}
	// Only known actions are accepted.
	_, err := conn.Exec(ctx, `INSERT INTO audit_events (owner_id, action) VALUES ($1, 'link.edited')`, owner.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("unknown action: err = %v, want a check violation (23514)", err)
	}
	if got, _ := st.AuditLog(ctx, owner.ID, 10); len(got) != 1 || got[0].Action != audit.KeyCreated {
		t.Fatalf("trail after tampering = %+v, want the one key.created event", got)
	}
}
