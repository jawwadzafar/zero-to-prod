package links

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Cache is an optional fast lookup in front of the Store (Redis in snip).
// A cache is allowed to forget anything at any time; the Store is the truth.
type Cache interface {
	GetURL(ctx context.Context, slug string) (url string, ok bool, err error)
	SetURL(ctx context.Context, slug, url string) error
	Delete(ctx context.Context, slug string) error
}

// ClickRecorder receives "someone followed this link" events. In snip it
// drops them on a queue for the worker; without a queue it counts directly.
type ClickRecorder interface {
	Record(ctx context.Context, slug string) error
}

// Service is snip's business logic: shorten, resolve, list, delete.
type Service struct {
	store  Store
	cache  Cache         // may be nil
	clicks ClickRecorder // may be nil
	now    func() time.Time
}

// NewService wires a Service. cache and clicks may be nil.
func NewService(store Store, cache Cache, clicks ClickRecorder) *Service {
	return &Service{store: store, cache: cache, clicks: clicks, now: time.Now}
}

// Shorten creates a link for rawURL. If slug is empty a random one is
// generated (retrying on the rare collision); otherwise the custom slug is
// validated and used as-is.
func (s *Service) Shorten(ctx context.Context, ownerID int64, rawURL, slug string) (Link, error) {
	u, err := NormalizeURL(rawURL)
	if err != nil {
		return Link{}, err
	}
	if slug != "" {
		if err := ValidateSlug(slug); err != nil {
			return Link{}, err
		}
		return s.store.Create(ctx, Link{Slug: slug, URL: u, OwnerID: ownerID, CreatedAt: s.now()})
	}
	for attempt := 0; attempt < 5; attempt++ {
		l, err := s.store.Create(ctx, Link{Slug: NewSlug(), URL: u, OwnerID: ownerID, CreatedAt: s.now()})
		if errors.Is(err, ErrSlugTaken) {
			continue
		}
		return l, err
	}
	return Link{}, fmt.Errorf("could not find a free slug after 5 attempts")
}

// ResolveResult says where a slug points and whether the cache answered.
type ResolveResult struct {
	URL      string
	CacheHit bool
}

// Resolve finds the URL for slug: cache first, then the store (filling the
// cache on the way out). Cache errors are never fatal — a broken cache makes
// snip slower, not broken.
func (s *Service) Resolve(ctx context.Context, slug string) (ResolveResult, error) {
	if s.cache != nil {
		if u, ok, err := s.cache.GetURL(ctx, slug); err == nil && ok {
			return ResolveResult{URL: u, CacheHit: true}, nil
		}
	}
	l, err := s.store.Get(ctx, slug)
	if err != nil {
		return ResolveResult{}, err
	}
	if s.cache != nil {
		_ = s.cache.SetURL(ctx, slug, l.URL) // best effort
	}
	return ResolveResult{URL: l.URL}, nil
}

// RecordClick reports a visit. Errors are returned so the caller can log
// them, but a failed click must never block the redirect.
func (s *Service) RecordClick(ctx context.Context, slug string) error {
	if s.clicks == nil {
		return s.store.AddClicks(ctx, slug, 1)
	}
	return s.clicks.Record(ctx, slug)
}

// Get returns a link's details, but only to its owner.
func (s *Service) Get(ctx context.Context, ownerID int64, slug string) (Link, error) {
	l, err := s.store.Get(ctx, slug)
	if err != nil {
		return Link{}, err
	}
	if l.OwnerID != ownerID {
		return Link{}, ErrNotFound // don't reveal that someone else's link exists
	}
	return l, nil
}

// List returns the owner's most recent links.
func (s *Service) List(ctx context.Context, ownerID int64, limit int) ([]Link, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.store.ListByOwner(ctx, ownerID, limit)
}

// Delete removes a link the owner created, and evicts it from the cache so
// redirects stop immediately.
func (s *Service) Delete(ctx context.Context, ownerID int64, slug string) error {
	if err := s.store.Delete(ctx, slug, ownerID); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, slug)
	}
	return nil
}
