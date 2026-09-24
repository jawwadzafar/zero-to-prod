// snip v0 — the whole link shortener in one file (chapter 5.1).
// Links live in a map in memory, so they vanish when the program stops.
//
//	go run ./examples/v0
//	curl -X POST localhost:8080/links -d 'https://go.dev'
//	curl -i localhost:8080/<code>
package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

var (
	mu    sync.Mutex            // protects links: many requests can arrive at once
	links = map[string]string{} // short code -> long URL
)

func main() {
	http.HandleFunc("POST /links", shorten) // create a short link
	http.HandleFunc("GET /{code}", follow)  // follow a short link
	log.Println("snip v0 listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func shorten(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2048))
	long := strings.TrimSpace(string(body))
	if !strings.HasPrefix(long, "http://") && !strings.HasPrefix(long, "https://") {
		http.Error(w, "send a URL starting with http:// or https://", http.StatusBadRequest)
		return
	}
	code := rand.Text()[:6] // random letters and digits
	mu.Lock()
	links[code] = long
	mu.Unlock()
	fmt.Fprintf(w, "http://localhost:8080/%s\n", code)
}

func follow(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	long, ok := links[r.PathValue("code")]
	mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, long, http.StatusFound)
}
