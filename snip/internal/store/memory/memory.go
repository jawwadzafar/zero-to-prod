// Package memory is a Store and KeyStore that keep everything in a Go map.
// It forgets everything on restart — perfect for tests and early chapters,
// useless for production (which is the lesson of chapter 6.1).
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

// Store is safe for concurrent use: many requests may hit it at once, so
// every access holds the mutex (chapter 4.5).
type Store struct {
	mu     sync.RWMutex
	links  map[string]links.Link
	keys   map[string]auth.Owner // keyHash -> owner
	nextID int64
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{links: map[string]links.Link{}, keys: map[string]auth.Owner{}}
}

func (s *Store) Create(_ context.Context, l links.Link) (links.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.links[l.Slug]; exists {
		return links.Link{}, links.ErrSlugTaken
	}
	s.links[l.Slug] = l
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

func (s *Store) Delete(_ context.Context, slug string, ownerID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[slug]
	if !ok || l.OwnerID != ownerID {
		return links.ErrNotFound
	}
	delete(s.links, slug)
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

func (s *Store) CreateKey(_ context.Context, name, keyHash string) (auth.Owner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	o := auth.Owner{ID: s.nextID, Name: name}
	s.keys[keyHash] = o
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
