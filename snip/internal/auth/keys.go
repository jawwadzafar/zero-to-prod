// Package auth handles snip's API keys: generating them, storing only a hash,
// and looking up who a presented key belongs to.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

// ErrUnknownKey means no active key matches.
var ErrUnknownKey = errors.New("unknown or revoked api key")

// Owner is whoever an API key belongs to.
type Owner struct {
	ID   int64
	Name string
}

// KeyStore persists API keys by their hash.
type KeyStore interface {
	CreateKey(ctx context.Context, name, keyHash string) (Owner, error)
	LookupKey(ctx context.Context, keyHash string) (Owner, error) // ErrUnknownKey if none
}

const keyPrefix = "snip_"
const keyChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateKey returns a new random API key such as "snip_9fQ…" (32 random
// characters ≈ 190 bits — far beyond guessing). Show it to the user once;
// store only HashKey(key).
func GenerateKey() string {
	var b strings.Builder
	b.WriteString(keyPrefix)
	max := big.NewInt(int64(len(keyChars)))
	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("crypto/rand unavailable: " + err.Error())
		}
		b.WriteByte(keyChars[n.Int64()])
	}
	return b.String()
}

// HashKey returns the SHA-256 of key, hex-encoded. A fast hash is fine here
// (unlike for passwords) because keys are long and random: there is nothing
// to brute-force. If the database leaks, the hashes are useless on their own.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CreateKey generates a key, stores its hash, and returns the plaintext key
// (the only time it is ever visible) with its owner record.
func CreateKey(ctx context.Context, ks KeyStore, name string) (string, Owner, error) {
	key := GenerateKey()
	owner, err := ks.CreateKey(ctx, name, HashKey(key))
	return key, owner, err
}

// Authenticate looks up the owner of a presented key.
func Authenticate(ctx context.Context, ks KeyStore, key string) (Owner, error) {
	if !strings.HasPrefix(key, keyPrefix) {
		return Owner{}, ErrUnknownKey
	}
	return ks.LookupKey(ctx, HashKey(key))
}
