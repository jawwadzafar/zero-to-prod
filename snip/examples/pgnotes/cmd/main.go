// Command pgnotes is a tiny notes app on Postgres (chapter 6.6):
//
//	export DATABASE_URL="postgres://postgres:learn@localhost:5432/postgres?sslmode=disable"
//	go run ./examples/pgnotes/cmd add groceries "eggs, flour"
//	go run ./examples/pgnotes/cmd list
//	go run ./examples/pgnotes/cmd get groceries
//	go run ./examples/pgnotes/cmd delete groceries
//	go run ./examples/pgnotes/cmd get "x' OR '1'='1"          # safe: not found
//	go run ./examples/pgnotes/cmd find-unsafe "x' OR '1'='1"  # unsafe: leaks every note
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/pgnotes"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: add TITLE BODY | get TITLE | list | delete TITLE | find-unsafe TITLE")
		os.Exit(2)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "set DATABASE_URL, e.g. postgres://postgres:learn@localhost:5432/postgres?sslmode=disable")
		os.Exit(2)
	}
	// Every database call gets a deadline: a hung query can't hang the program.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notes, err := pgnotes.Open(ctx, url)
	if err != nil {
		fail(err)
	}
	defer notes.Close()
	if err := notes.Setup(ctx); err != nil {
		fail(err)
	}

	arg := func(i int) string {
		if len(os.Args) <= i {
			fail(fmt.Errorf("%s needs more arguments", os.Args[1]))
		}
		return os.Args[i]
	}
	switch os.Args[1] {
	case "add":
		n, err := notes.Add(ctx, arg(2), arg(3))
		if errors.Is(err, pgnotes.ErrDuplicate) {
			fail(fmt.Errorf("%q: %w", arg(2), err))
		}
		if err != nil {
			fail(err)
		}
		fmt.Printf("added note %d at %s\n", n.ID, n.CreatedAt.Format(time.RFC3339))
	case "get":
		n, err := notes.Get(ctx, arg(2))
		if err != nil {
			fail(err)
		}
		show(n)
	case "list":
		all, err := notes.List(ctx, 20)
		if err != nil {
			fail(err)
		}
		for _, n := range all {
			show(n)
		}
	case "delete":
		if err := notes.Delete(ctx, arg(2)); err != nil {
			fail(err)
		}
		fmt.Println("deleted")
	case "find-unsafe":
		found, err := notes.FindUnsafe(ctx, arg(2))
		if err != nil {
			fail(err)
		}
		fmt.Printf("string-built query returned %d note(s):\n", len(found))
		for _, n := range found {
			show(n)
		}
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func show(n pgnotes.Note) {
	fmt.Printf("%-4d %-12s %s\n", n.ID, n.Title, n.Body)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
