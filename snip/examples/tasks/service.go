package main

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// --- Service layer: the rules of the business. No HTTP, no SQL. -------------

// ErrInvalidTitle is a rule about tasks, so it lives here, not in the handler.
var ErrInvalidTitle = errors.New("title must be 1-200 characters")

// Service holds the business rules. It is handed a Repository from outside
// (dependency injection), so tests can hand it a fake.
type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Create checks the rules, then asks the repository to store the task.
func (s *Service) Create(ctx context.Context, title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return Task{}, ErrInvalidTitle
	}
	return s.repo.Create(ctx, Task{Title: title, CreatedAt: s.now().UTC()})
}

func (s *Service) Get(ctx context.Context, id int64) (Task, error) {
	return s.repo.Get(ctx, id)
}

// List returns the newest tasks. The page size is a business decision:
// at most 100, 20 if the caller doesn't say.
func (s *Service) List(ctx context.Context, limit int) ([]Task, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.List(ctx, limit)
}
