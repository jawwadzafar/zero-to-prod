package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/postgres"
)

// newLogin creates a throwaway login holding role, as "snip migrate" does,
// and drops it when the test ends. It returns the login's connection URL.
func newLogin(t *testing.T, owner *postgres.Store, role string) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("SNIP_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	user := fmt.Sprintf("%s_test_%d", role, time.Now().UnixNano())
	u.User = url.UserPassword(user, fmt.Sprintf("pw-%d", time.Now().UnixNano()))
	got, err := owner.EnsureLogin(context.Background(), u.String(), role)
	if err != nil || got != user {
		t.Fatalf("EnsureLogin = %q, %v", got, err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if admin, err := pgx.Connect(ctx, os.Getenv("SNIP_TEST_DATABASE_URL")); err == nil {
			_, _ = admin.Exec(ctx, `DROP ROLE `+pgx.Identifier{user}.Sanitize())
			admin.Close(ctx)
		}
	})
	return u.String()
}

// refused runs each statement and wants Postgres's own privilege error
// ("permission denied", "must be owner"), not the audit trigger's exception
// (which uses the same code): the point is that the login lacks the right.
func refused(t *testing.T, conn *pgx.Conn, stmts ...string) {
	t.Helper()
	for _, sql := range stmts {
		_, err := conn.Exec(context.Background(), sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" || strings.Contains(pgErr.Message, "append-only") {
			t.Errorf("%s: err = %v, want a privilege error (42501) from Postgres", sql, err)
		}
	}
}

// The app's login (snip_app, migration 0003) does all of snip's work but
// can't change the schema, drop the audit trigger, or rewrite history.
func TestAppLoginIsLeastPrivilege(t *testing.T) {
	owner := openTestDB(t)
	ctx := context.Background()
	appURL := newLogin(t, owner, postgres.AppRole)

	app, err := postgres.Open(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if applied, err := app.Migrate(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("Migrate as the app = %v, %v; want nothing to do and no error", applied, err)
	}

	// Its normal work: keys, links, clicks, deletes, the audit trail.
	key, o, err := auth.CreateKey(ctx, app, "least-privilege")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, app, key); err != nil {
		t.Fatal(err)
	}
	svc := links.NewService(app, nil, nil)
	l, err := svc.Shorten(ctx, o.ID, "https://www.postgresql.org/docs/current/ddl-priv.html", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddClicks(ctx, l.Slug, 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, o.ID, l.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RotateKey(ctx, app, o.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.RevokeKey(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if trail, err := app.AuditLog(ctx, o.ID, 10); err != nil || len(trail) != 5 {
		t.Fatalf("AuditLog = %d events, %v; want 5", len(trail), err)
	}

	// What it may not do.
	conn, err := pgx.Connect(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	refused(t, conn,
		`DROP TRIGGER audit_events_append_only ON audit_events`,
		`ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only`,
		`DROP FUNCTION audit_events_append_only()`,
		`UPDATE audit_events SET action = 'key.created'`,
		`DELETE FROM audit_events`,
		`TRUNCATE audit_events`,
		`DROP TABLE audit_events`,
		`ALTER TABLE links ADD COLUMN note TEXT`,
		`DROP TABLE links`,
		`TRUNCATE links`,
		`CREATE TABLE stash (x INT)`,
		`DELETE FROM schema_migrations`,
	)
}

// The read-only login reads what analytics needs, never a key hash, and
// writes nothing.
func TestReadonlyLoginOnlyReads(t *testing.T) {
	owner := openTestDB(t)
	ctx := context.Background()
	if _, _, err := auth.CreateKey(ctx, owner, "reader-test"); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, newLogin(t, owner, postgres.ReadonlyRole))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, sql := range []string{
		`SELECT count(*) FROM links`,
		`SELECT count(*) FROM audit_events`,
		`SELECT count(id), count(name), count(created_at), count(revoked_at) FROM api_keys`,
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Errorf("%s: %v", sql, err)
		}
	}
	refused(t, conn,
		`SELECT key_hash FROM api_keys`,
		`INSERT INTO audit_events (owner_id, action) VALUES (1, 'key.created')`,
		`UPDATE links SET clicks = 0`,
		`DELETE FROM links`,
	)
}

// EnsureLogin sets the password on every run, so changing it in the URL and
// running "snip migrate" again rotates it. It refuses the owner's own login.
func TestEnsureLoginRotatesAndRefusesTheOwner(t *testing.T) {
	owner := openTestDB(t)
	ctx := context.Background()
	oldURL := newLogin(t, owner, postgres.AppRole)
	u, _ := url.Parse(oldURL)
	u.User = url.UserPassword(u.User.Username(), "a-new-password")
	if _, err := owner.EnsureLogin(ctx, u.String(), postgres.AppRole); err != nil {
		t.Fatal(err)
	}
	if c, err := pgx.Connect(ctx, oldURL); err == nil {
		c.Close(ctx)
		t.Fatal("the old password still works after rotation")
	}
	c, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatalf("new password: %v", err)
	}
	c.Close(ctx)

	if _, err := owner.EnsureLogin(ctx, os.Getenv("SNIP_TEST_DATABASE_URL"), postgres.AppRole); err == nil {
		t.Fatal("EnsureLogin accepted the owner's own login")
	}
	if _, err := owner.EnsureLogin(ctx, "postgres://nopassword@localhost/snip", postgres.AppRole); err == nil {
		t.Fatal("EnsureLogin accepted a URL without a password")
	}
}
