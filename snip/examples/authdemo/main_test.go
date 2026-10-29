package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSessionsAndKeys(t *testing.T) {
	srv := httptest.NewTLSServer(newServer().routes()) // TLS, so the Secure cookie is sent
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := srv.Client()
	c.Jar = jar

	// Wrong password and unknown user get the same answer.
	for _, form := range []url.Values{
		{"user": {"ada"}, "password": {"wrong"}},
		{"user": {"nobody"}, "password": {"wrong"}},
	} {
		resp, _ := c.PostForm(srv.URL+"/login", form)
		if got := body(t, resp); resp.StatusCode != 401 || !strings.Contains(got, "invalid user or password") {
			t.Fatalf("bad login: %d %q", resp.StatusCode, got)
		}
	}

	resp, _ := c.PostForm(srv.URL+"/login", url.Values{"user": {"ada"}, "password": {"correct horse battery staple"}})
	body(t, resp)
	cookie := resp.Header.Get("Set-Cookie")
	for _, flag := range []string{"HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(cookie, flag) {
			t.Fatalf("cookie %q lacks %s", cookie, flag)
		}
	}
	resp, _ = c.Get(srv.URL + "/me")
	if got := body(t, resp); !strings.Contains(got, "ada (via session cookie)") {
		t.Fatalf("me: %q", got)
	}

	// Create a key; it is printed once and works as a bearer token.
	resp, _ = c.Post(srv.URL+"/keys", "", nil)
	out := body(t, resp)
	key := strings.Fields(strings.SplitN(out, "\n", 2)[0])[2]
	if !strings.HasPrefix(key, "demo_") {
		t.Fatalf("key %q", key)
	}
	anon := &http.Client{Transport: srv.Client().Transport} // no cookie jar: a script with only the key
	me := func() (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+"/me", nil)
		req.Header.Set("Authorization", "bearer "+key) // scheme is case-insensitive
		resp, err := anon.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, body(t, resp)
	}
	if code, got := me(); code != 200 || !strings.Contains(got, "via API key 1") {
		t.Fatalf("key auth: %d %q", code, got)
	}

	// Revoke: refused from the very next request.
	req, _ := http.NewRequest("DELETE", srv.URL+"/keys/1", nil)
	resp, _ = c.Do(req)
	body(t, resp)
	if code, _ := me(); code != 401 {
		t.Fatalf("revoked key still works: %d", code)
	}

	// Log out: the session is gone on the server.
	resp, _ = c.Post(srv.URL+"/logout", "", nil)
	body(t, resp)
	resp, _ = c.Get(srv.URL + "/me")
	if resp.StatusCode != 401 {
		t.Fatalf("session survived logout: %d", resp.StatusCode)
	}
	body(t, resp)
}

func TestKeyIsNotStored(t *testing.T) {
	s := newServer()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := srv.Client()
	c.Jar = jar
	resp, _ := c.PostForm(srv.URL+"/login", url.Values{"user": {"ada"}, "password": {"correct horse battery staple"}})
	body(t, resp)
	resp, _ = c.Post(srv.URL+"/keys", "", nil)
	key := strings.Fields(body(t, resp))[2]
	for h := range s.keys {
		if strings.Contains(h, key) {
			t.Fatal("the key itself must not be stored")
		}
	}
	if s.keys[hashKey(key)] == nil {
		t.Fatal("hash of the key should be stored")
	}
}
