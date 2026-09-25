package totp

import (
	"testing"
	"time"
)

var rfcSecret = []byte("12345678901234567890") // the secret both RFCs use

// RFC 4226, appendix D: the first ten HOTP values for that secret.
func TestHOTPMatchesRFC4226(t *testing.T) {
	want := []string{"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489"}
	for i, w := range want {
		if got := HOTP(rfcSecret, uint64(i), 6); got != w {
			t.Errorf("counter %d: got %s, want %s", i, got, w)
		}
	}
}

// RFC 6238, appendix B: 8-digit TOTP values with SHA-1 at fixed times.
func TestTOTPMatchesRFC6238(t *testing.T) {
	cases := map[int64]string{
		59:          "94287082",
		1111111109:  "07081804",
		1111111111:  "14050471",
		1234567890:  "89005924",
		2000000000:  "69279037",
		20000000000: "65353130",
	}
	for unix, want := range cases {
		if got := TOTP(rfcSecret, time.Unix(unix, 0), 8); got != want {
			t.Errorf("time %d: got %s, want %s", unix, got, want)
		}
	}
}

func TestVerifierAcceptsDriftAndRefusesReplay(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	v := &Verifier{Secret: rfcSecret, Skew: 1}

	previous := TOTP(rfcSecret, now.Add(-Step), 6) // the phone is 30 s slow
	if !v.Verify(previous, now) {
		t.Fatal("a code from the previous step should be accepted")
	}
	if v.Verify(previous, now) {
		t.Fatal("the same code must not be accepted twice")
	}
	if v.Verify(TOTP(rfcSecret, now.Add(-5*Step), 6), now) {
		t.Fatal("a code from 2.5 minutes ago must be refused")
	}
	if v.Verify("12345", now) || v.Verify("abcdef", now) {
		t.Fatal("malformed codes must be refused")
	}
	if !v.Verify(TOTP(rfcSecret, now, 6), now) {
		t.Fatal("the current code should be accepted")
	}
}

func TestURIIsWhatAppsExpect(t *testing.T) {
	got := URI(rfcSecret, "snip", "ada@example.com")
	want := "otpauth://totp/snip:ada@example.com?digits=6&issuer=snip&period=30&secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
