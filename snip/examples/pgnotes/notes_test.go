package pgnotes

import (
	"context"
	"errors"
	"os"
	"testing"
)

// open connects to SNIP_TEST_DATABASE_URL and starts from an empty table.
func open(t *testing.T) *Notes {
	t.Helper()
	url := os.Getenv("SNIP_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SNIP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	n, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	if err := n.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := n.pool.Exec(ctx, `TRUNCATE notes`); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddGetListDelete(t *testing.T) {
	n := open(t)
	ctx := context.Background()
	added, err := n.Add(ctx, "groceries", "eggs, flour")
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == 0 || added.CreatedAt.IsZero() {
		t.Fatalf("RETURNING did not fill id and created_at: %+v", added)
	}
	got, err := n.Get(ctx, "groceries")
	if err != nil || got.Body != "eggs, flour" {
		t.Fatalf("Get: %+v, %v", got, err)
	}
	if _, err := n.Add(ctx, "ideas", "learn SQL"); err != nil {
		t.Fatal(err)
	}
	list, err := n.List(ctx, 10)
	if err != nil || len(list) != 2 || list[0].Title != "ideas" {
		t.Fatalf("List: %+v, %v", list, err)
	}
	if err := n.Delete(ctx, "groceries"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Get(ctx, "groceries"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v, want ErrNotFound", err)
	}
	if err := n.Delete(ctx, "groceries"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v, want ErrNotFound", err)
	}
}

func TestDuplicateTitleBecomesErrDuplicate(t *testing.T) {
	n := open(t)
	ctx := context.Background()
	if _, err := n.Add(ctx, "todo", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Add(ctx, "todo", "b"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("got %v, want ErrDuplicate", err)
	}
}

func TestPlaceholderStopsInjection(t *testing.T) {
	n := open(t)
	ctx := context.Background()
	for _, title := range []string{"private", "secret"} {
		if _, err := n.Add(ctx, title, "not for you"); err != nil {
			t.Fatal(err)
		}
	}
	attack := "x' OR '1'='1"
	if _, err := n.Get(ctx, attack); !errors.Is(err, ErrNotFound) {
		t.Fatalf("placeholder version: %v, want ErrNotFound", err)
	}
	leaked, err := n.FindUnsafe(ctx, attack)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) != 2 {
		t.Fatalf("string-built version returned %d notes; the demo expects it to leak both", len(leaked))
	}
}
