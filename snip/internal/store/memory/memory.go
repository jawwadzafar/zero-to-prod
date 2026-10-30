// Package memory is a Store and KeyStore that keep everything in a Go map.
// It forgets everything on restart — perfect for tests and early chapters,
// useless for production (which is the lesson of chapter 6.1).
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/audit"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

// Store is safe for concurrent use: many requests may hit it at once, so
// every access holds the mutex (chapter 4.5). Each change appends to events,
// the in-memory audit trail, while still holding the lock, so a change and
// its event happen together, like the Postgres store's transaction.
type Store struct {
	mu     sync.RWMutex
	links  map[string]links.Link
	keys   map[string]auth.Owner // keyHash -> owner
	nextID int64
	events []audit.Event // append-only: nothing in this file edits or removes one
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{links: map[string]links.Link{}, keys: map[string]auth.Owner{}}
}

func (s *Store) Create(ctx context.Context, l links.Link) (links.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.links[l.Slug]; exists {
		return links.Link{}, links.ErrSlugTaken
	}
	s.links[l.Slug] = l
	s.record(ctx, l.OwnerID, audit.LinkCreated, l.Slug, map[string]string{"url": l.URL})
	return l, nil
}

func (s *Store) Get(_ context.Context, slug string) (links.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.links[slug]
	if !ok {
		return links.Link{}, links.ErrNotFound
	}
	return l, nil
}

func (s *Store) ListByOwner(_ context.Context, ownerID int64, limit int) ([]links.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []links.Link
	for _, l := range s.links {
		if l.OwnerID == ownerID {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, slug string, ownerID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[slug]
	if !ok || l.OwnerID != ownerID {
		return links.ErrNotFound
	}
	delete(s.links, slug)
	s.record(ctx, ownerID, audit.LinkDeleted, slug, map[string]string{"url": l.URL})
	return nil
}

func (s *Store) AddClicks(_ context.Context, slug string, n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[slug]
	if !ok {
		return links.ErrNotFound
	}
	l.Clicks += n
	s.links[slug] = l
	return nil
}

func (s *Store) CreateKey(ctx context.Context, name, keyHash string) (auth.Owner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	o := auth.Owner{ID: s.nextID, Name: name}
	s.keys[keyHash] = o
	s.record(ctx, o.ID, audit.KeyCreated, "", map[string]string{"name": name})
	return o, nil
}

func (s *Store) LookupKey(_ context.Context, keyHash string) (auth.Owner, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.keys[keyHash]
	if !ok {
		return auth.Owner{}, auth.ErrUnknownKey
	}
	return o, nil
}

func (s *Store) RevokeKey(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, o := range s.keys {
		if o.ID == id {
			delete(s.keys, h)
			s.record(ctx, id, audit.KeyRevoked, "", nil)
			return nil
		}
	}
	return auth.ErrUnknownKey
}

func (s *Store) ReplaceKey(ctx context.Context, id int64, keyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, o := range s.keys {
		if o.ID == id {
			delete(s.keys, h)
			s.keys[keyHash] = o
			s.record(ctx, id, audit.KeyRotated, "", nil)
			return nil
		}
	}
	return auth.ErrUnknownKey
}

// record appends an audit event. The caller holds s.mu.
func (s *Store) record(ctx context.Context, ownerID int64, action audit.Action, slug string, detail map[string]string) {
	s.events = append(s.events, audit.Event{
		ID: int64(len(s.events) + 1), At: time.Now(), OwnerID: ownerID, Action: action,
		Slug: slug, RequestID: audit.RequestID(ctx), Detail: audit.DetailFor(ctx, detail),
	})
}

// AuditLog returns ownerID's most recent events, newest first.
func (s *Store) AuditLog(_ context.Context, ownerID int64, limit int) ([]audit.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []audit.Event
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		if s.events[i].OwnerID == ownerID {
			out = append(out, s.events[i])
		}
	}
	return out, nil
}
