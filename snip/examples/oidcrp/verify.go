// Package oidcrp is a tiny OpenID Connect relying party: the part of an app
// that lets people "Sign in with <your company's identity provider>"
// (chapter 7.9). Standard library only, so every check is visible. In
// production, use a maintained library; this code exists to be read.
package oidcrp

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// Provider is what discovery tells us about an identity provider.
type Provider struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`

	client      *http.Client
	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey // by key ID ("kid")
	lastRefresh time.Time
}

// Discover reads the provider's /.well-known/openid-configuration and its
// public signing keys.
func Discover(ctx context.Context, client *http.Client, issuer string) (*Provider, error) {
	var p Provider
	if err := getJSON(ctx, client, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration", &p); err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	// The document must describe the issuer we asked for, or anyone who can
	// serve a document could claim to be our identity provider.
	if p.Issuer != issuer {
		return nil, fmt.Errorf("discovery: issuer %q, want %q", p.Issuer, issuer)
	}
	p.client = client
	if err := p.refreshKeys(ctx); err != nil {
		return nil, err
	}
	return &p, nil
}

// refreshKeys (re)loads the provider's public signing keys. Providers rotate
// keys, so this runs at start and again whenever a token names a key we
// haven't seen.
func (p *Provider) refreshKeys(ctx context.Context) error {
	var set struct {
		Keys []struct {
			Kty, Kid, Use, Alg, N, E string
		} `json:"keys"`
	}
	if err := getJSON(ctx, p.client, p.JWKSURI, &set); err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return errors.New("jwks: no usable RSA signing keys")
	}
	p.mu.Lock()
	p.keys, p.lastRefresh = keys, time.Now()
	p.mu.Unlock()
	return nil
}

// key finds a signing key, refetching the key set once if the ID is unknown.
// Refetches are limited to one every 10 seconds, so a flood of tokens with made-up
// key IDs can't turn us into a tool for hammering the provider.
func (p *Provider) key(kid string) (*rsa.PublicKey, bool) {
	p.mu.Lock()
	k, ok := p.keys[kid]
	recent := time.Since(p.lastRefresh) < 10*time.Second
	p.mu.Unlock()
	if ok || recent || p.client == nil {
		return k, ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.refreshKeys(ctx) != nil {
		return nil, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k, ok = p.keys[kid]
	return k, ok
}

func getJSON(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Claims are the ID token fields a relying party must check or use.
type Claims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"` // the stable user ID: key accounts on (iss, sub), never on email
	Audience          audience `json:"aud"`
	AuthorizedParty   string   `json:"azp"`
	Expiry            int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
	Nonce             string   `json:"nonce"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	PreferredUsername string   `json:"preferred_username"`
	AMR               []string `json:"amr"` // how the user authenticated, when the provider says
	ACR               string   `json:"acr"`
}

// audience accepts both forms the spec allows: "a" and ["a", "b"].
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// Errors, so tests and callers can tell the failures apart.
var (
	ErrMalformed   = errors.New("token is not a well-formed JWT")
	ErrAlgorithm   = errors.New("token algorithm not allowed")
	ErrUnknownKey  = errors.New("token signed with an unknown key")
	ErrSignature   = errors.New("token signature is invalid")
	ErrIssuer      = errors.New("token from the wrong issuer")
	ErrAudience    = errors.New("token meant for a different client")
	ErrExpired     = errors.New("token expired")
	ErrNonce       = errors.New("token nonce does not match this login")
	ErrNotYetValid = errors.New("token issued in the future")
)

// VerifyIDToken performs the checks from OpenID Connect Core, section 3.1.3.7,
// in order. Every one of them has been skipped by a real product, and every
// skipped one has been exploited.
func (p *Provider) VerifyIDToken(raw, clientID, nonce string, now time.Time) (*Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
	}
	var header struct{ Alg, Kid string }
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, ErrMalformed
	}
	// 1. Only the algorithm we expect. Never "none", and never an HMAC
	//    algorithm keyed with the public key ("algorithm confusion").
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("%w: %q", ErrAlgorithm, header.Alg)
	}
	// 2. A key we fetched from the provider, and the signature checks out.
	key, ok := p.key(header.Kid)
	if !ok {
		return nil, ErrUnknownKey
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrMalformed
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return nil, ErrSignature
	}
	// Only now is it safe to trust anything inside the token.
	var c Claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return nil, ErrMalformed
	}
	// 3. Issued by our provider...
	if c.Issuer != p.Issuer {
		return nil, ErrIssuer
	}
	// 4. ...for us, not for some other app that uses the same provider.
	if !slices.Contains(c.Audience, clientID) || (len(c.Audience) > 1 && c.AuthorizedParty != clientID) {
		return nil, ErrAudience
	}
	// 5. Still valid (with a minute of leeway for clock differences).
	const leeway = 60
	if now.Unix() > c.Expiry+leeway {
		return nil, ErrExpired
	}
	if c.IssuedAt > now.Unix()+leeway {
		return nil, ErrNotYetValid
	}
	// 6. Created for this login attempt, not replayed from another one.
	if nonce != "" && c.Nonce != nonce {
		return nil, ErrNonce
	}
	return &c, nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
