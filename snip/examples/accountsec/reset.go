// Package accountsec holds two account-security building blocks (chapter 7.8):
// password-reset tokens done properly, and a breached-password check that
// never sends the password anywhere. Standard library only.
package accountsec

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrInvalidToken is the only error a caller sees for a bad token: whether it
// never existed, expired or was already used, the answer is the same, so an
// attacker learns nothing from trying.
var ErrInvalidToken = errors.New("invalid or expired reset link")

type resetRecord struct {
	userID  string
	expires time.Time
}

// Resets issues and redeems password-reset tokens.
//
//   - The token is 32 random bytes: unguessable (chapter 7.1).
//   - Only its SHA-256 hash is stored, so a leaked database can't be used to
//     reset anyone's password (the same idea as snip's API keys).
//   - It expires quickly and works exactly once.
//   - Issuing a new one cancels the user's older ones.
type Resets struct {
	TTL time.Duration
	Now func() time.Time

	mu     sync.Mutex
	byHash map[string]resetRecord
}

func NewResets(ttl time.Duration) *Resets {
	return &Resets{TTL: ttl, Now: time.Now, byHash: map[string]resetRecord{}}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Issue returns a token to put in the reset link. Send it only to the
// address already on the account, never to one given in the request.
func (r *Resets) Issue(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	r.mu.Lock()
	defer r.mu.Unlock()
	for h, rec := range r.byHash {
		if rec.userID == userID {
			delete(r.byHash, h) // one live link per user
		}
	}
	r.byHash[hashToken(token)] = resetRecord{userID: userID, expires: r.Now().Add(r.TTL)}
	return token, nil
}

// Redeem checks a token and returns the user it belongs to, consuming it.
// After a successful reset, the caller should also end all of that user's
// sessions: whoever triggered the reset may be the only legitimate one.
func (r *Resets) Redeem(token string) (string, error) {
	h := hashToken(token)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byHash[h] // looking up by hash: no timing leak on the token itself
	if !ok {
		return "", ErrInvalidToken
	}
	delete(r.byHash, h) // single use, even if it turns out to be expired
	if r.Now().After(rec.expires) {
		return "", ErrInvalidToken
	}
	return rec.userID, nil
}
