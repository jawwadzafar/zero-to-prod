package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Signing and verifying in the Standard Webhooks format
// (https://www.standardwebhooks.com):
//
//	webhook-id:        msg_2KWPBgLlAfxdpx2AI54pPJ85f4W   unique per message, the same on every retry
//	webhook-timestamp: 1790000000                        Unix seconds when this attempt was signed
//	webhook-signature: v1,K5oZfzN95Z9UVu1EsfQmfVNQhnkZ2pj9o9NDN/H/pI4=
//
// The signature is HMAC-SHA256 over "id.timestamp.body", keyed with a shared
// secret, base64-encoded. Several signatures may be sent, separated by
// spaces, so a secret can be rotated without dropping events.

const secretPrefix = "whsec_"

// NewSecret makes a random 32-byte secret, written the standard way.
func NewSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the OS has no randomness: nothing sensible to do
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(b)
}

// ParseSecret turns "whsec_<base64>" into the raw key bytes.
func ParseSecret(s string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, secretPrefix))
	if err != nil || len(key) < 24 {
		return nil, errors.New("secret must be whsec_ followed by at least 24 base64-encoded bytes")
	}
	return key, nil
}

// Sign returns the signature value for one message, e.g. "v1,K5oZ…=".
func Sign(key []byte, id string, ts time.Time, body []byte) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%d.", id, ts.Unix())
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// SignHeaders sets all three headers on an outgoing request. With more than
// one key (during a rotation) it sends one signature per key.
func SignHeaders(h http.Header, keys [][]byte, id string, ts time.Time, body []byte) {
	sigs := make([]string, len(keys))
	for i, k := range keys {
		sigs[i] = Sign(k, id, ts, body)
	}
	h.Set("webhook-id", id)
	h.Set("webhook-timestamp", strconv.FormatInt(ts.Unix(), 10))
	h.Set("webhook-signature", strings.Join(sigs, " "))
}

var (
	ErrMissingHeaders = errors.New("missing webhook-id, webhook-timestamp or webhook-signature")
	ErrTimestamp      = errors.New("timestamp outside the allowed window (possible replay)")
	ErrSignature      = errors.New("no signature matches")
)

// Verify checks a received message. It accepts the message if ANY of the
// signatures matches ANY of the keys (old and new, during a rotation), and
// only if the timestamp is within tolerance of now.
func Verify(keys [][]byte, h http.Header, body []byte, now time.Time, tolerance time.Duration) error {
	id, tsRaw, sigHeader := h.Get("webhook-id"), h.Get("webhook-timestamp"), h.Get("webhook-signature")
	if id == "" || tsRaw == "" || sigHeader == "" {
		return ErrMissingHeaders
	}
	secs, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return ErrTimestamp
	}
	ts := time.Unix(secs, 0)
	if now.Sub(ts) > tolerance || ts.Sub(now) > tolerance { // too old, or from the future
		return ErrTimestamp
	}
	for _, k := range keys {
		want := Sign(k, id, ts, body)
		for _, got := range strings.Fields(sigHeader) {
			// hmac.Equal takes the same time whether the first byte or the
			// last one differs, so an attacker can't guess a signature byte
			// by byte from response times. Never compare signatures with ==.
			if hmac.Equal([]byte(got), []byte(want)) {
				return nil
			}
		}
	}
	return ErrSignature
}
