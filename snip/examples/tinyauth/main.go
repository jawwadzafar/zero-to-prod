// tinyauth — a toy OAuth 2.0 authorization server AND a protected API, in one
// program, using only Go's standard library (chapter 7.3). It exists to show
// every moving part. For real systems, use a real identity provider.
//
//	go run ./examples/tinyauth
//
//	# 1. discovery document and public keys (anyone may read these)
//	curl -s localhost:9000/.well-known/openid-configuration
//	curl -s localhost:9000/jwks.json
//
//	# 2. a machine gets a token with the client-credentials grant
//	curl -s -X POST localhost:9000/token -d grant_type=client_credentials \
//	     -d client_id=snip-worker -d client_secret=worker-secret -d scope=links:read
//
//	# 3. it calls the protected API with the token
//	curl -s localhost:9000/api/links -H "Authorization: Bearer <access_token>"
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	issuer   = "http://localhost:9000" // who issues tokens (the "iss" claim)
	audience = "snip-api"              // who tokens are for (the "aud" claim)
	keyID    = "tinyauth-key-1"        // lets verifiers pick the right public key
)

// Registered clients. A real server stores hashed secrets in a database.
var clients = map[string]struct {
	secret string
	scopes []string
}{
	"snip-worker": {secret: "worker-secret", scopes: []string{"links:read", "links:write"}},
	"dashboard":   {secret: "dashboard-secret", scopes: []string{"links:read"}},
}

var privateKey ed25519.PrivateKey
var publicKey ed25519.PublicKey

func main() {
	var err error
	publicKey, privateKey, err = ed25519.GenerateKey(rand.Reader) // new keys each start: old tokens stop verifying
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("GET /.well-known/openid-configuration", discovery)
	http.HandleFunc("GET /jwks.json", jwks)
	http.HandleFunc("POST /token", token)
	http.HandleFunc("GET /api/links", requireScope("links:read", listLinks))
	log.Println("tinyauth on", issuer)
	log.Fatal(http.ListenAndServe(":9000", nil))
}

// discovery tells clients where everything is (OpenID Connect Discovery).
func discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"issuer":                                issuer,
		"token_endpoint":                        issuer + "/token",
		"jwks_uri":                              issuer + "/jwks.json",
		"grant_types_supported":                 []string{"client_credentials"},
		"id_token_signing_alg_values_supported": []string{"EdDSA"},
	})
}

// jwks publishes the PUBLIC key, so anyone can verify tokens — but only the
// holder of the private key can create them.
func jwks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"keys": []map[string]string{{
		"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": keyID,
		"x": b64(publicKey),
	}}})
}

// token implements the client-credentials grant: a program proves who it is
// with its client ID and secret, and receives a short-lived access token.
func token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "client_credentials" {
		writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	id, secret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	c, ok := clients[id]
	// Compare secrets in constant time so response timing leaks nothing.
	if !ok || subtle.ConstantTimeCompare([]byte(secret), []byte(c.secret)) != 1 {
		writeJSON(w, 401, map[string]string{"error": "invalid_client"})
		return
	}
	var granted []string
	for _, s := range strings.Fields(r.PostForm.Get("scope")) {
		if slices.Contains(c.scopes, s) { // only scopes this client is allowed
			granted = append(granted, s)
		}
	}
	now := time.Now()
	claims := map[string]any{
		"iss":   issuer,
		"sub":   id,
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   now.Add(5 * time.Minute).Unix(), // short-lived
		"scope": strings.Join(granted, " "),
	}
	writeJSON(w, 200, map[string]any{
		"access_token": sign(claims),
		"token_type":   "Bearer",
		"expires_in":   300,
		"scope":        claims["scope"],
	})
}

// sign builds a JWT: base64url(header).base64url(claims).base64url(signature).
func sign(claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": keyID})
	payload, _ := json.Marshal(claims)
	unsigned := b64(header) + "." + b64(payload)
	return unsigned + "." + b64(ed25519.Sign(privateKey, []byte(unsigned)))
}

// verify checks the signature, then the claims that make a token valid HERE.
func verify(tok string) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	var header map[string]string
	if err := decodeJSON(parts[0], &header); err != nil || header["alg"] != "EdDSA" || header["kid"] != keyID {
		return nil, errors.New("unexpected algorithm or key") // never trust "alg": "none"
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, errors.New("bad signature")
	}
	var claims map[string]any
	if err := decodeJSON(parts[1], &claims); err != nil {
		return nil, err
	}
	if claims["iss"] != issuer || claims["aud"] != audience {
		return nil, errors.New("wrong issuer or audience")
	}
	if exp, _ := claims["exp"].(float64); time.Now().Unix() >= int64(exp) {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

// requireScope protects a handler: valid token AND the needed scope.
func requireScope(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims, err := verify(tok)
		if !ok || err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeJSON(w, 401, map[string]string{"error": "invalid_token"})
			return
		}
		if s, _ := claims["scope"].(string); !slices.Contains(strings.Fields(s), scope) {
			writeJSON(w, 403, map[string]string{"error": "insufficient_scope", "needed": scope})
			return
		}
		next(w, r)
	}
}

func listLinks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"links": []string{"godoc", "launch"}})
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func decodeJSON(part string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
