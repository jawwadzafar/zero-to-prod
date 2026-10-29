// Package testdemo is a small, standalone example for chapter 4.4: one pure
// function, one function with a dependency, and one HTTP handler, each with
// tests.
//
//	go test -v ./examples/testdemo/
//	go test -cover ./examples/testdemo/
package testdemo

import (
	"strings"
	"unicode"
)

// Slugify turns a title into a URL-friendly slug:
// "Hello, World!" becomes "hello-world".
func Slugify(title string) string {
	var b strings.Builder
	dash := false // did we just write a dash? (avoids "a--b")
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
