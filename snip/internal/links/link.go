// Package links holds snip's core idea — a short slug that points at a long
// URL — with no knowledge of HTTP, databases or caches. Everything else in
// snip plugs into the interfaces defined here.
package links

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Link is one shortened URL.
type Link struct {
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	OwnerID   int64     `json:"-"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

// Errors callers can check with errors.Is. They describe what went wrong in
// domain terms; the HTTP layer decides which status code each one becomes.
var (
	ErrNotFound    = errors.New("link not found")
	ErrSlugTaken   = errors.New("slug already taken")
	ErrInvalidURL  = errors.New("invalid url")
	ErrInvalidSlug = errors.New("invalid slug")
)

// Store is anything that can keep links: memory for tests and early chapters,
// Postgres for real use. The service code never knows which one it has.
// Create and Delete also append an audit event (package audit) as part of
// the same change, so no link appears or disappears without a record.
type Store interface {
	Create(ctx context.Context, l Link) (Link, error) // ErrSlugTaken if the slug exists
	Get(ctx context.Context, slug string) (Link, error)
	ListByOwner(ctx context.Context, ownerID int64, limit int) ([]Link, error)
	Delete(ctx context.Context, slug string, ownerID int64) error // ErrNotFound if missing or not theirs
	AddClicks(ctx context.Context, slug string, n int64) error
}

const maxURLLength = 2048

// NormalizeURL checks that raw is an absolute http(s) URL and returns it
// trimmed. We refuse other schemes (javascript:, file:, data:) because a
// shortener that redirects to them becomes a tool for attackers, and URLs
// with a username or password in them: "https://yourbank.com@evil.example/"
// goes to evil.example, a classic disguise for phishing links.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	// A URL may not contain spaces (clients percent-encode them as %20).
	// Without this check "http://x? #" came out as "http://x? ", with a raw
	// space that would end up in a redirect's Location header. The nightly
	// fuzz job found it (chapter 7.13).
	if raw == "" || len(raw) > maxURLLength || strings.ContainsFunc(raw, unicode.IsSpace) {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", ErrInvalidURL
	}
	// Check the length again after normalising: u.String() percent-encodes
	// bytes such as "é" (2 bytes) into "%C3%A9" (6), so a URL that passed the
	// first check can come out longer than the database column allows. A fuzz
	// test found this (chapter 7.13).
	out := u.String()
	if len(out) > maxURLLength {
		return "", ErrInvalidURL
	}
	return out, nil
}

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// reserved are slugs that would collide with snip's own routes, or with
// the web UI that nginx serves at /app/ (chapter 16.6).
var reserved = map[string]bool{"api": true, "app": true, "healthz": true, "readyz": true, "metrics": true}

// ValidateSlug checks a custom slug chosen by a user.
func ValidateSlug(s string) error {
	if !slugPattern.MatchString(s) || reserved[strings.ToLower(s)] {
		return ErrInvalidSlug
	}
	return nil
}

const (
	alphabet   = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/l/I: easy to read aloud
	slugLength = 7
)

// NewSlug returns a random 7-character slug. With 56 possible characters that
// is 56^7 ≈ 1.7 trillion combinations, so collisions are rare — and the
// service retries if one happens anyway.
func NewSlug() string {
	b := make([]byte, slugLength)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("crypto/rand unavailable: " + err.Error()) // the OS random source never fails in practice
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}
