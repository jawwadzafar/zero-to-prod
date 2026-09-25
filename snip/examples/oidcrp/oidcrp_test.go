package oidcrp

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeIdP serves discovery and a key set, and signs tokens like a real
// identity provider would.
type fakeIdP struct {
	srv *httptest.Server
	key *rsa.PrivateKey
	kid string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, kid: "k1"}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 f.srv.URL,
			"authorization_endpoint": f.srv.URL + "/auth",
			"token_endpoint":         f.srv.URL + "/token",
			"jwks_uri":               f.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.E)).Bytes()),
		}}})
	})
	t.Cleanup(f.srv.Close)
	return f
}

func seg(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (f *fakeIdP) sign(header, claims map[string]any) string {
	signing := seg(header) + "." + seg(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f *fakeIdP) claims(now time.Time) map[string]any {
	return map[string]any{
		"iss": f.srv.URL, "sub": "user-123", "aud": "snip-web",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
		"nonce": "n-1", "email": "ada@example.com", "email_verified": true,
	}
}

func TestVerifyIDToken(t *testing.T) {
	f := newFakeIdP(t)
	p, err := Discover(context.Background(), f.srv.Client(), f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_790_000_000, 0)
	rs256 := map[string]any{"alg": "RS256", "kid": "k1", "typ": "JWT"}
	good := f.sign(rs256, f.claims(now))

	c, err := p.VerifyIDToken(good, "snip-web", "n-1", now)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if c.Subject != "user-123" || !c.EmailVerified {
		t.Fatalf("claims: %+v", c)
	}

	with := func(key string, v any) string {
		cl := f.claims(now)
		cl[key] = v
		return f.sign(rs256, cl)
	}
	parts := strings.Split(good, ".")
	tampered := f.claims(now)
	tampered["sub"] = "admin"
	// HS256 "signed" with the public key's bytes: the classic algorithm-confusion attack.
	confusion := seg(map[string]any{"alg": "HS256", "kid": "k1"}) + "." + seg(f.claims(now))
	mac := hmac.New(sha256.New, f.key.N.Bytes())
	mac.Write([]byte(confusion))
	confusion += "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	cases := map[string]struct {
		token string
		want  error
	}{
		"payload changed after signing": {parts[0] + "." + seg(tampered) + "." + parts[2], ErrSignature},
		"alg none":                      {seg(map[string]any{"alg": "none"}) + "." + parts[1] + ".", ErrAlgorithm},
		"HS256 algorithm confusion":     {confusion, ErrAlgorithm},
		"unknown key id":                {f.sign(map[string]any{"alg": "RS256", "kid": "other"}, f.claims(now)), ErrUnknownKey},
		"other app's token":             {with("aud", "another-app"), ErrAudience},
		"wrong issuer":                  {with("iss", "https://evil.example"), ErrIssuer},
		"expired":                       {with("exp", now.Add(-10*time.Minute).Unix()), ErrExpired},
		"replayed from another login":   {with("nonce", "n-other"), ErrNonce},
		"not a JWT":                     {"hello", ErrMalformed},
	}
	for name, tc := range cases {
		if _, err := p.VerifyIDToken(tc.token, "snip-web", "n-1", now); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestDiscoveryRejectsAMismatchedIssuer(t *testing.T) {
	f := newFakeIdP(t)
	if _, err := Discover(context.Background(), f.srv.Client(), f.srv.URL+"/other"); err == nil {
		t.Fatal("a discovery document for a different issuer must be rejected")
	}
}

func TestAuthURLCarriesPKCEStateAndNonce(t *testing.T) {
	p := &Provider{AuthorizationEndpoint: "https://idp.example/auth"}
	a := NewAttempt()
	u := p.AuthURL(a, "snip-web", "http://localhost:8765/callback")
	for _, want := range []string{"code_challenge_method=S256", "state=" + a.State, "nonce=" + a.Nonce, "scope=openid"} {
		if !strings.Contains(u, want) {
			t.Errorf("auth URL lacks %q: %s", want, u)
		}
	}
	if strings.Contains(u, a.CodeVerifier) {
		t.Error("the PKCE verifier must never be sent to the browser, only its hash")
	}
}

func TestKeyRotationIsPickedUp(t *testing.T) {
	f := newFakeIdP(t)
	p, err := Discover(context.Background(), f.srv.Client(), f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// The provider rotates to a new key; tokens now name "k2".
	f.key, _ = rsa.GenerateKey(rand.Reader, 2048)
	f.kid = "k2"
	token := f.sign(map[string]any{"alg": "RS256", "kid": "k2"}, f.claims(now))

	p.lastRefresh = time.Time{} // pretend the last refresh was long ago
	if _, err := p.VerifyIDToken(token, "snip-web", "n-1", now); err != nil {
		t.Fatalf("after a key rotation the new key should be fetched: %v", err)
	}
	// A made-up key ID straight after a refresh doesn't trigger another fetch.
	bogus := f.sign(map[string]any{"alg": "RS256", "kid": "made-up"}, f.claims(now))
	if _, err := p.VerifyIDToken(bogus, "snip-web", "n-1", now); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("got %v, want ErrUnknownKey", err)
	}
}
