// Slices, maps, structs, methods, pointers and errors (chapter 4.2).
//
//	go run ./examples/structs
package main

import (
	"errors"
	"fmt"
	"strings"
)

// Link groups the facts about one short link into a single value.
type Link struct {
	Slug   string
	URL    string
	Clicks int
}

// Describe is a method: a function attached to the Link type.
func (l Link) Describe() string {
	return fmt.Sprintf("/%s -> %s (%d clicks)", l.Slug, l.URL, l.Clicks)
}

// AddClick needs a pointer receiver (*Link) because it changes the link.
func (l *Link) AddClick() {
	l.Clicks++
}

// ErrNotFound is a sentinel error: a named error callers can check for.
var ErrNotFound = errors.New("link not found")

// Store keeps links in a map from slug to Link.
type Store struct {
	links map[string]Link
}

func NewStore() *Store {
	return &Store{links: map[string]Link{}}
}

func (s *Store) Save(l Link) error {
	if !strings.HasPrefix(l.URL, "https://") {
		return fmt.Errorf("save %q: url must start with https:// (got %q)", l.Slug, l.URL)
	}
	s.links[l.Slug] = l
	return nil
}

func (s *Store) Get(slug string) (Link, error) {
	l, ok := s.links[slug]
	if !ok {
		return Link{}, fmt.Errorf("get %q: %w", slug, ErrNotFound) // %w wraps: keeps ErrNotFound inside
	}
	return l, nil
}

func main() {
	// A slice: an ordered, growable list.
	slugs := []string{"go", "gh"}
	slugs = append(slugs, "ex")
	fmt.Println("slugs:", slugs, "count:", len(slugs), "first:", slugs[0])

	// A map: look values up by key.
	owners := map[string]string{"go": "ada", "gh": "grace"}
	owners["ex"] = "linus"
	owner, ok := owners["nope"]
	fmt.Printf("owner of nope: %q (found: %v)\n", owner, ok)

	// Structs and methods.
	l := Link{Slug: "godoc", URL: "https://go.dev/doc/"}
	l.AddClick()
	l.AddClick()
	fmt.Println(l.Describe())

	// Errors are values: check them, wrap them, and test for specific ones.
	store := NewStore()
	if err := store.Save(Link{Slug: "bad", URL: "ftp://files"}); err != nil {
		fmt.Println("error:", err)
	}
	_ = store.Save(l)
	if got, err := store.Get("godoc"); err == nil {
		fmt.Println("found:", got.Describe())
	}
	_, err := store.Get("missing")
	fmt.Println("error:", err)
	fmt.Println("is it ErrNotFound?", errors.Is(err, ErrNotFound))
}
