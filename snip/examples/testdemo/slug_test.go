package testdemo

import "testing"

// A table-driven test: one row per case, one loop that checks them all.
func TestSlugify(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"lowercases", "Hello", "hello"},
		{"spaces become dashes", "hello world", "hello-world"},
		{"punctuation is dropped", "Hello, World!", "hello-world"},
		{"no double dashes", "a  --  b", "a-b"},
		{"no dash at the ends", "  -go-  ", "go"},
		{"digits are kept", "Top 10 tips", "top-10-tips"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slugify(tt.in); got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
