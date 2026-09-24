package links

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// tracer creates spans for the steps inside a request (chapter 13.3). When
// tracing is off, the global provider is a no-op and spans cost almost nothing.
var tracer = otel.Tracer("github.com/jawwadzafar/zero-to-prod/snip/internal/links")

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
	ctx, span := tracer.Start(ctx, "links.Resolve")
	defer span.End()

	cacheBroken := false
	if s.cache != nil {
		cctx, cspan := tracer.Start(ctx, "cache.GetURL")
		u, ok, err := s.cache.GetURL(cctx, slug)
		cspan.SetAttributes(attribute.Bool("cache.hit", err == nil && ok))
		if err != nil {
			cspan.RecordError(err)
			cacheBroken = true
		}
		cspan.End()
		if err == nil && ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return ResolveResult{URL: u, CacheHit: true}, nil
		}
	}
	sctx, sspan := tracer.Start(ctx, "store.Get")
	l, err := s.store.Get(sctx, slug)
	if err != nil && !errors.Is(err, ErrNotFound) {
		sspan.RecordError(err)
		sspan.SetStatus(codes.Error, "store lookup failed")
	}
	sspan.End()
	if err != nil {
		return ResolveResult{}, err
	}
	// If the cache just failed, don't wait on it a second time: during an
	// outage every call costs a timeout (chapter 13.5).
	if s.cache != nil && !cacheBroken {
		cctx, cspan := tracer.Start(ctx, "cache.SetURL")
		_ = s.cache.SetURL(cctx, slug, l.URL) // best effort
		cspan.End()
	}
	span.SetAttributes(attribute.Bool("cache.hit", false))
	return ResolveResult{URL: l.URL}, nil
}

// RecordClick reports a visit. Errors are returned so the caller can log
// them, but a failed click must never block the redirect.
func (s *Service) RecordClick(ctx context.Context, slug string) error {
	ctx, span := tracer.Start(ctx, "links.RecordClick")
	defer span.End()
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
