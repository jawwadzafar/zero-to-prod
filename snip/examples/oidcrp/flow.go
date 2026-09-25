package oidcrp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Attempt is one login in progress, remembered between the redirect to the
// provider and the redirect back.
type Attempt struct {
	State        string // ties the callback to this browser (CSRF protection)
	Nonce        string // ties the ID token to this login (replay protection)
	CodeVerifier string // PKCE: proves the code is redeemed by whoever started the login
}

// NewAttempt creates the three secrets for one login.
func NewAttempt() Attempt {
	return Attempt{State: random(), Nonce: random(), CodeVerifier: random()}
}

// AuthURL is where to send the browser to log in.
func (p *Provider) AuthURL(a Attempt, clientID, redirectURI string, scopes ...string) string {
	challenge := sha256.Sum256([]byte(a.CodeVerifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(append([]string{"openid"}, scopes...), " ")},
		"state":                 {a.State},
		"nonce":                 {a.Nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	return p.AuthorizationEndpoint + "?" + q.Encode()
}

// Tokens is the token endpoint's answer.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Exchange trades the one-time code for tokens, from the server side, with
// the PKCE verifier and the client's credentials.
func (p *Provider) Exchange(ctx context.Context, client *http.Client, a Attempt, code, clientID, clientSecret, redirectURI string) (*Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {a.CodeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(clientSecret))
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return nil, fmt.Errorf("token endpoint: %s: %s %s", res.Status, e.Error, e.Description)
	}
	var t Tokens
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
		return nil, err
	}
	if t.IDToken == "" {
		return nil, errors.New("token endpoint returned no id_token (was the openid scope requested?)")
	}
	return &t, nil
}
