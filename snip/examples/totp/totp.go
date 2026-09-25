// Package totp implements one-time passwords: HOTP (RFC 4226) and TOTP
// (RFC 6238), the codes authenticator apps show (chapter 7.7). Standard
// library only, small enough to read in one sitting.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 and every authenticator app use HMAC-SHA1; HMAC keeps it safe here
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Step is how long each code is valid: 30 seconds, as authenticator apps expect.
const Step = 30 * time.Second

// HOTP computes the code for one counter value (RFC 4226, section 5.3).
func HOTP(secret []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil) // 20 bytes

	// "Dynamic truncation": the last 4 bits pick where to read 4 bytes from.
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod)
}

// Counter is the number of 30-second steps since the Unix epoch.
func Counter(t time.Time) uint64 {
	return uint64(t.Unix()) / uint64(Step.Seconds())
}

// TOTP is HOTP with the counter taken from the clock (RFC 6238).
func TOTP(secret []byte, t time.Time, digits int) string {
	return HOTP(secret, Counter(t), digits)
}

// NewSecret returns a random 20-byte secret, the size RFC 4226 recommends.
func NewSecret() ([]byte, error) {
	s := make([]byte, 20)
	_, err := rand.Read(s)
	return s, err
}

// Base32 is how secrets are shown to people and put in QR codes.
func Base32(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// URI is the otpauth:// link an authenticator app scans as a QR code.
func URI(secret []byte, issuer, account string) string {
	v := url.Values{}
	v.Set("secret", Base32(secret))
	v.Set("issuer", issuer)
	v.Set("digits", "6")
	v.Set("period", "30")
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

// Verifier checks codes for one user. It accepts the current step and one
// step either side (phone clocks drift), and refuses a step it has already
// accepted, so a code seen over someone's shoulder can't be replayed.
type Verifier struct {
	Secret   []byte
	Skew     int    // steps accepted either side of now; 1 is usual
	LastUsed uint64 // highest counter accepted so far; store it with the user
}

// Verify reports whether code is valid at time now, and records its use.
func (v *Verifier) Verify(code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	current := Counter(now)
	for d := -v.Skew; d <= v.Skew; d++ {
		c := uint64(int64(current) + int64(d))
		if c <= v.LastUsed {
			continue // already used (or older than a used one): replay
		}
		want := HOTP(v.Secret, c, 6)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			v.LastUsed = c
			return true
		}
	}
	return false
}
