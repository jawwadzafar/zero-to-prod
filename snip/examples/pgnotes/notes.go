// Package pgnotes is the smallest useful Go program that talks to Postgres
// (chapter 6.6): a connection pool, a table created at startup, the three
// ways to run SQL (QueryRow, Query, Exec), placeholders, and turning database
// errors into errors the rest of your program understands.
//
// It stores notes: a title (unique) and a body. Nothing here knows about
// snip; chapter 6.6 applies the same moves to snip's real store afterwards.
package pgnotes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Note is one row of the notes table, as a Go value.
type Note struct {
	ID        int64
	Title     string
	Body      string
	CreatedAt time.Time
}

// The errors callers check for. They say what happened in this program's
// words, so nothing outside this file needs to know about pgx or SQLSTATE codes.
var (
	ErrNotFound  = errors.New("note not found")
	ErrDuplicate = errors.New("a note with that title already exists")
)

// Notes is a store of notes in Postgres, backed by a connection pool.
type Notes struct {
	pool *pgxpool.Pool
}

// Open creates a pool for databaseURL (postgres://user:pass@host:5432/db)
// and checks that the database answers, so a wrong password fails now rather
// than on the first real query.
func Open(ctx context.Context, databaseURL string) (*Notes, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.MaxConns = 5 // at most 5 connections open at once; requests beyond that wait their turn
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Notes{pool: pool}, nil
}

// Close closes every pooled connection.
func (n *Notes) Close() { n.pool.Close() }

// Setup creates the table if it isn't there yet. A real service uses
// numbered migration files instead (chapter 6.6 shows how).
func (n *Notes) Setup(ctx context.Context) error {
	_, err := n.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS notes (
		id         BIGSERIAL   PRIMARY KEY,
		title      TEXT        NOT NULL UNIQUE,
		body       TEXT        NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	return err
}

// uniqueViolation is the SQLSTATE code Postgres sends for a duplicate key.
const uniqueViolation = "23505"

// Add inserts a note. RETURNING sends back the values Postgres filled in
// (the id and the timestamp) in the same round trip.
func (n *Notes) Add(ctx context.Context, title, body string) (Note, error) {
	note := Note{Title: title, Body: body}
	err := n.pool.QueryRow(ctx,
		`INSERT INTO notes (title, body) VALUES ($1, $2) RETURNING id, created_at`,
		title, body,
	).Scan(&note.ID, &note.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return Note{}, ErrDuplicate
	}
	return note, err
}

// Get returns the note with this title: QueryRow, for exactly one row.
func (n *Notes) Get(ctx context.Context, title string) (Note, error) {
	var note Note
	err := n.pool.QueryRow(ctx,
		`SELECT id, title, body, created_at FROM notes WHERE title = $1`, title,
	).Scan(&note.ID, &note.Title, &note.Body, &note.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return note, err
}

// List returns up to limit notes, newest first: Query, for many rows.
// CollectRows scans every row and closes the result, which hands the
// connection back to the pool.
func (n *Notes) List(ctx context.Context, limit int) ([]Note, error) {
	rows, err := n.pool.Query(ctx,
		`SELECT id, title, body, created_at FROM notes ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanNote)
}

// Delete removes the note with this title: Exec, when no rows come back.
func (n *Notes) Delete(ctx context.Context, title string) error {
	tag, err := n.pool.Exec(ctx, `DELETE FROM notes WHERE title = $1`, title)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindUnsafe is the WRONG way, kept so you can watch it fail (chapter 6.6
// lab): it glues the title into the SQL text. A title such as
// x' OR '1'='1 rewrites the query and returns every note. Never do this.
func (n *Notes) FindUnsafe(ctx context.Context, title string) ([]Note, error) {
	query := "SELECT id, title, body, created_at FROM notes WHERE title = '" + title + "'"
	rows, err := n.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanNote)
}

func scanNote(r pgx.CollectableRow) (Note, error) {
	var note Note
	err := r.Scan(&note.ID, &note.Title, &note.Body, &note.CreatedAt)
	return note, err
}
