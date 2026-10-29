// authdemo — passwords, sessions and API keys in one small program
// (chapter 7.2). Standard library only. It keeps everything in memory and
// listens only on 127.0.0.1: a teaching tool, not something to deploy.
//
//	go run ./examples/authdemo
//
//	# 1. log in with a password: the server sets a session cookie
//	curl -si -c jar.txt -X POST localhost:9200/login -d user=ada -d password='correct horse battery staple'
//	# 2. the browser (here: curl's cookie jar) sends the cookie back
//	curl -s -b jar.txt localhost:9200/me
//	# 3. while logged in, create an API key: shown once, only its hash is kept
//	curl -s -b jar.txt -X POST localhost:9200/keys
//	# 4. a script uses the key as a bearer token
//	curl -s localhost:9200/me -H "Authorization: Bearer demo_..."
//	# 5. revoke the key, log out: both stop working at once
//	curl -s -b jar.txt -X DELETE localhost:9200/keys/1
//	curl -s -b jar.txt -X POST localhost:9200/logout
package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A stored password: a random salt and a deliberately slow hash (chapter 7.1).
// PBKDF2-SHA256 with 600,000 iterations is OWASP's current minimum for PBKDF2;
// Argon2id or bcrypt are better choices when you can add a dependency.
type password struct {
	salt []byte
	hash []byte
}

const iterations = 600_000

func hashPassword(pw string, salt []byte) []byte {
	h, err := pbkdf2.Key(sha256.New, pw, salt, iterations, 32)
	if err != nil {
		panic(err)
	}
	return h
}

func newPassword(pw string) password {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	return password{salt: salt, hash: hashPassword(pw, salt)}
}

type apiKey struct {
	id      int
	user    string
	revoked bool
}

type server struct {
	mu        sync.Mutex
	passwords map[string]password // user → salted slow hash
	sessions  map[string]string   // session ID → user (a real app uses Redis or a table)
	keys      map[string]*apiKey  // SHA-256 of key → key record; the key itself is never stored
	nextKeyID int
}

func newServer() *server {
	return &server{
		passwords: map[string]password{"ada": newPassword("correct horse battery staple")},
		sessions:  map[string]string{},
		keys:      map[string]*apiKey{},
		nextKeyID: 1,
	}
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		user, pw := r.FormValue("user"), r.FormValue("password")
		s.mu.Lock()
		stored, ok := s.passwords[user]
		s.mu.Unlock()
		if !ok {
			// Do the same slow work for unknown users, so response time doesn't
			// reveal which accounts exist.
			stored = password{salt: make([]byte, 16), hash: make([]byte, 32)}
		}
		match := subtle.ConstantTimeCompare(hashPassword(pw, stored.salt), stored.hash) == 1
		if !ok || !match {
			http.Error(w, "invalid user or password", http.StatusUnauthorized) // same message either way
			return
		}
		id := rand.Text() // ~128 random bits: the session ID is a temporary password
		s.mu.Lock()
		s.sessions[id] = user
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    id,
			Path:     "/",
			MaxAge:   int((7 * 24 * time.Hour).Seconds()),
			HttpOnly: true,                 // page scripts can't read it
			Secure:   true,                 // HTTPS only (browsers and curl allow localhost)
			SameSite: http.SameSiteLaxMode, // not sent on most cross-site requests
		})
		fmt.Fprintf(w, "logged in as %s\n", user)
	})

	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		user, how := s.whoami(r)
		if user == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="authdemo"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, "you are %s (via %s)\n", user, how)
	})

	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err == nil {
			s.mu.Lock()
			delete(s.sessions, c.Value) // server-side: invalid everywhere, at once
			s.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1})
		fmt.Fprintln(w, "logged out")
	})

	// Creating and revoking keys needs a logged-in person (a session), not a key.
	mux.HandleFunc("POST /keys", func(w http.ResponseWriter, r *http.Request) {
		user := s.sessionUser(r)
		if user == "" {
			http.Error(w, "log in first", http.StatusUnauthorized)
			return
		}
		key := "demo_" + rand.Text() // prefix: easy for secret scanners to spot
		s.mu.Lock()
		k := &apiKey{id: s.nextKeyID, user: user}
		s.nextKeyID++
		s.keys[hashKey(key)] = k
		s.mu.Unlock()
		fmt.Fprintf(w, "key %d: %s\n(shown once: only its SHA-256 is stored)\n", k.id, key)
	})

	mux.HandleFunc("DELETE /keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		user := s.sessionUser(r)
		id, _ := strconv.Atoi(r.PathValue("id"))
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, k := range s.keys {
			if k.id == id && k.user == user && user != "" {
				k.revoked = true // keep the record, refuse the key from now on
				fmt.Fprintf(w, "key %d revoked\n", id)
				return
			}
		}
		http.Error(w, "no such key", http.StatusNotFound)
	})

	return mux
}

func (s *server) sessionUser(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[c.Value]
}

// whoami accepts either a session cookie (people in browsers) or a bearer
// API key (scripts and programs).
func (s *server) whoami(r *http.Request) (user, how string) {
	if u := s.sessionUser(r); u != "" {
		return u, "session cookie"
	}
	scheme, key, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.keys[hashKey(strings.TrimSpace(key))] // hash what was sent, look up the hash
	if k == nil || k.revoked {
		return "", ""
	}
	return k.user, "API key " + strconv.Itoa(k.id)
}

func main() {
	log.Println("authdemo on http://localhost:9200 — local only; user ada, password 'correct horse battery staple'")
	log.Fatal(http.ListenAndServe("127.0.0.1:9200", newServer().routes()))
}
