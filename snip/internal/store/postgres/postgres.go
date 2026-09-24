// Package postgres is snip's real Store and KeyStore, backed by PostgreSQL
// through a connection pool (pgxpool).
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to databaseURL (e.g. postgres://user:pass@host:5432/db) and
// checks the connection works.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases every pooled connection.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database is reachable (used by /readyz).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies any migrations/*.sql files not yet recorded in
// schema_migrations, in filename order, each inside its own transaction.
// It returns the versions it applied.
func (s *Store) Migrate(ctx context.Context) ([]int, error) {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	// Serialise concurrent migrators (e.g. two pods starting at once).
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(7412)`); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7412)`) //nolint:errcheck

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	applied := []int{} // empty, not nil, so "nothing to do" logs as []
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		version, err := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
		if err != nil {
			return applied, fmt.Errorf("migration %s: name must start with a number", base)
		}
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return applied, err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return applied, err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("migration %s: %w", base, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return applied, err
		}
		if err := tx.Commit(ctx); err != nil {
			return applied, err
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// uniqueViolation is Postgres's error code for a duplicate key.
const uniqueViolation = "23505"

func (s *Store) Create(ctx context.Context, l links.Link) (links.Link, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO links (slug, url, owner_id, created_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING clicks, created_at`,
		l.Slug, l.URL, l.OwnerID, l.CreatedAt,
	).Scan(&l.Clicks, &l.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return links.Link{}, links.ErrSlugTaken
	}
	return l, err
}

func (s *Store) Get(ctx context.Context, slug string) (links.Link, error) {
	var l links.Link
	err := s.pool.QueryRow(ctx,
		`SELECT slug, url, owner_id, clicks, created_at FROM links WHERE slug = $1`, slug,
	).Scan(&l.Slug, &l.URL, &l.OwnerID, &l.Clicks, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return links.Link{}, links.ErrNotFound
	}
	return l, err
}

func (s *Store) ListByOwner(ctx context.Context, ownerID int64, limit int) ([]links.Link, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT slug, url, owner_id, clicks, created_at
		   FROM links
		  WHERE owner_id = $1
		  ORDER BY created_at DESC
		  LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (links.Link, error) {
		var l links.Link
		err := r.Scan(&l.Slug, &l.URL, &l.OwnerID, &l.Clicks, &l.CreatedAt)
		return l, err
	})
}

func (s *Store) Delete(ctx context.Context, slug string, ownerID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM links WHERE slug = $1 AND owner_id = $2`, slug, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return links.ErrNotFound
	}
	return nil
}

// AddClicks adds n in one atomic UPDATE, so concurrent workers never lose a
// count (chapter 6.5 explains why "read, add, write back" would).
func (s *Store) AddClicks(ctx context.Context, slug string, n int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE links SET clicks = clicks + $2 WHERE slug = $1`, slug, n)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return links.ErrNotFound
	}
	return nil
}

func (s *Store) CreateKey(ctx context.Context, name, keyHash string) (auth.Owner, error) {
	o := auth.Owner{Name: name}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING id`, name, keyHash,
	).Scan(&o.ID)
	return o, err
}

func (s *Store) LookupKey(ctx context.Context, keyHash string) (auth.Owner, error) {
	var o auth.Owner
	err := s.pool.QueryRow(ctx,
		`SELECT id, name FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, keyHash,
	).Scan(&o.ID, &o.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Owner{}, auth.ErrUnknownKey
	}
	return o, err
}
