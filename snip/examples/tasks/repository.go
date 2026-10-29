package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// --- Repository layer: storage, and nothing else. ---------------------------

// Task is the one thing this API manages.
type Task struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrNotFound means "no task with that ID". The HTTP layer turns it into 404.
var ErrNotFound = errors.New("task not found")

// Repository is everything the service needs from storage. A map today; a
// database tomorrow, with no change to the service or the handlers.
type Repository interface {
	Create(ctx context.Context, t Task) (Task, error)
	Get(ctx context.Context, id int64) (Task, error)
	List(ctx context.Context, limit int) ([]Task, error)
}

// MemoryRepo keeps tasks in a map. Delay pretends every call is a slow
// database query, so you can watch request deadlines work.
type MemoryRepo struct {
	Delay time.Duration

	mu    sync.Mutex
	next  int64
	tasks map[int64]Task
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{tasks: map[int64]Task{}}
}

// wait sleeps for Delay, but gives up as soon as the request's context is
// cancelled (client gone) or its deadline passes — like a real database driver.
func (m *MemoryRepo) wait(ctx context.Context) error {
	if m.Delay == 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(m.Delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *MemoryRepo) Create(ctx context.Context, t Task) (Task, error) {
	if err := m.wait(ctx); err != nil {
		return Task{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	t.ID = m.next
	m.tasks[t.ID] = t
	return t, nil
}

func (m *MemoryRepo) Get(ctx context.Context, id int64) (Task, error) {
	if err := m.wait(ctx); err != nil {
		return Task{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return t, nil
}

func (m *MemoryRepo) List(ctx context.Context, limit int) ([]Task, error) {
	if err := m.wait(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Task{}
	for id := m.next; id > 0 && len(out) < limit; id-- { // newest first
		if t, ok := m.tasks[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}
