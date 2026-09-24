// A deliberately vulnerable page, and its fix (chapter 7.5).
// It only listens on 127.0.0.1 — never deploy it.
//
//	go run ./examples/xss
//	open  http://127.0.0.1:9100/unsafe?name=Ada
//	open  http://127.0.0.1:9100/unsafe?name=<script>alert('hacked')</script>
//	open  http://127.0.0.1:9100/safe?name=<script>alert('hacked')</script>
package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

var page = template.Must(template.New("hello").Parse(
	`<!doctype html><title>Hello</title><h1>Hello, {{.}}!</h1><p>(this page escapes its input)</p>`))

func main() {
	// UNSAFE: user input is pasted straight into HTML. Whatever HTML or
	// JavaScript the input contains becomes part of the page.
	http.HandleFunc("GET /unsafe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><title>Hello</title><h1>Hello, %s!</h1><p>(this page is vulnerable)</p>",
			r.URL.Query().Get("name"))
	})

	// SAFE: html/template escapes the input for the place it appears, so
	// <script> is shown as text instead of being run.
	http.HandleFunc("GET /safe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'") // a second layer: no inline scripts
		_ = page.Execute(w, r.URL.Query().Get("name"))
	})

	log.Println("vulnerable demo on http://127.0.0.1:9100 — local only")
	log.Fatal(http.ListenAndServe("127.0.0.1:9100", nil))
}
