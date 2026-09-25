// Command oidcrp is a tiny website with "Sign in with SSO" (chapter 7.9).
// Point it at any OpenID Connect provider; the lab uses a local Keycloak.
//
//	OIDC_ISSUER=http://localhost:8180/realms/snip OIDC_CLIENT_ID=snip-web \
//	OIDC_CLIENT_SECRET=... go run ./examples/oidcrp/cmd
//	# then open http://localhost:8765
package main

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/oidcrp"
)

type session struct {
	claims  *oidcrp.Claims
	expires time.Time
}

type app struct {
	p                          *oidcrp.Provider
	client                     *http.Client
	clientID, secret, redirect string
	mu                         sync.Mutex
	attempts                   map[string]oidcrp.Attempt // by state
	sessions                   map[string]session        // by session cookie
}

func main() {
	issuer := os.Getenv("OIDC_ISSUER")
	if issuer == "" {
		log.Fatal("set OIDC_ISSUER, OIDC_CLIENT_ID and OIDC_CLIENT_SECRET")
	}
	listen := cmp.Or(os.Getenv("LISTEN"), "localhost:8765")
	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	p, err := oidcrp.Discover(ctx, client, issuer)
	cancel() // discovery is done; the context isn't needed any more
	if err != nil {
		log.Fatal(err)
	}
	a := &app{
		p: p, client: client,
		clientID: os.Getenv("OIDC_CLIENT_ID"), secret: os.Getenv("OIDC_CLIENT_SECRET"),
		redirect: cmp.Or(os.Getenv("OIDC_REDIRECT_URL"), "http://"+listen+"/callback"),
		attempts: map[string]oidcrp.Attempt{}, sessions: map[string]session{},
	}
	http.HandleFunc("GET /{$}", a.home)
	http.HandleFunc("GET /login", a.login)
	http.HandleFunc("GET /callback", a.callback)
	http.HandleFunc("POST /logout", a.logout)
	log.Printf("relying party for %s on http://%s", issuer, listen)
	srv := &http.Server{Addr: listen, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	s, ok := a.current(r)
	a.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		fmt.Fprint(w, `<p>Not signed in.</p><p><a href="/login">Sign in with SSO</a></p>`)
		return
	}
	c := s.claims
	fmt.Fprintf(w, `<p>Signed in as <b>%s</b></p><ul><li>subject (stable ID): %s</li><li>issuer: %s</li>
<li>email: %s (verified: %t)</li><li>how you authenticated (amr): %v, level (acr): %s</li></ul>
<form method="post" action="/logout"><button>Sign out</button></form>`,
		html.EscapeString(c.PreferredUsername), html.EscapeString(c.Subject), html.EscapeString(c.Issuer),
		html.EscapeString(c.Email), c.EmailVerified, c.AMR, html.EscapeString(c.ACR))
}

// current returns the caller's session; the caller holds a.mu.
func (a *app) current(r *http.Request) (session, bool) {
	ck, err := r.Cookie("rp_session")
	if err != nil {
		return session{}, false
	}
	s, ok := a.sessions[ck.Value]
	if !ok || time.Now().After(s.expires) {
		return session{}, false
	}
	return s, true
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	at := oidcrp.NewAttempt()
	a.mu.Lock()
	a.attempts[at.State] = at
	a.mu.Unlock()
	http.Redirect(w, r, a.p.AuthURL(at, a.clientID, a.redirect, "profile", "email"), http.StatusFound)
}

func (a *app) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "login failed: "+e, http.StatusUnauthorized)
		return
	}
	a.mu.Lock()
	at, ok := a.attempts[q.Get("state")]
	delete(a.attempts, q.Get("state")) // each attempt can be completed once
	a.mu.Unlock()
	if !ok {
		http.Error(w, "unknown or reused login attempt (state mismatch)", http.StatusBadRequest)
		return
	}
	tokens, err := a.p.Exchange(r.Context(), a.client, at, q.Get("code"), a.clientID, a.secret, a.redirect)
	if err != nil {
		log.Printf("exchange: %v", err)
		http.Error(w, "could not complete login", http.StatusBadGateway)
		return
	}
	claims, err := a.p.VerifyIDToken(tokens.IDToken, a.clientID, at.Nonce, time.Now())
	if err != nil {
		log.Printf("id token rejected: %v", err)
		http.Error(w, "could not verify your identity", http.StatusUnauthorized)
		return
	}
	// Our own session from here on: a fresh random ID (never reuse one from
	// before login: session fixation, chapter 7.8), HttpOnly, SameSite.
	id := oidcrp.NewAttempt().State
	a.mu.Lock()
	a.sessions[id] = session{claims: claims, expires: time.Now().Add(8 * time.Hour)}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "rp_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	log.Printf("signed in: sub=%s user=%s", claims.Subject, claims.PreferredUsername)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie("rp_session"); err == nil {
		a.mu.Lock()
		delete(a.sessions, ck.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "rp_session", Value: "", Path: "/", MaxAge: -1})
	// Ending our session doesn't end the one at the identity provider; a real
	// app would also redirect to a.p.EndSessionEndpoint (single logout).
	http.Redirect(w, r, "/", http.StatusFound)
}
