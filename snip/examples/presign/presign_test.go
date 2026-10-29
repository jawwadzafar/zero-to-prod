package presign

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// AWS's published example for presigned URLs ("Authenticating Requests:
// Using Query Parameters", Amazon S3 API reference): same inputs, same signature.
func TestMatchesAWSExample(t *testing.T) {
	c := Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}
	now := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	got, err := URL("GET", "https://examplebucket.s3.amazonaws.com/test.txt", c, "us-east-1", now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	const wantSig = "aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if !strings.HasSuffix(got, "&X-Amz-Signature="+wantSig) {
		t.Fatalf("signature mismatch:\n got %s\nwant suffix %s", got, wantSig)
	}
}

func TestSignatureCoversMethodAndObject(t *testing.T) {
	c := Credentials{AccessKeyID: "test", SecretAccessKey: "secret"}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	sig := func(method, raw string) string {
		s, err := URL(method, raw, c, "us-east-1", now, 15*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(s)
		return u.Query().Get("X-Amz-Signature")
	}
	base := sig("PUT", "http://localhost:8333/uploads/cat.png")
	if base == sig("GET", "http://localhost:8333/uploads/cat.png") {
		t.Error("changing the method must change the signature")
	}
	if base == sig("PUT", "http://localhost:8333/uploads/dog.png") {
		t.Error("changing the object key must change the signature")
	}
}

func TestEncodesKeysWithSpaces(t *testing.T) {
	c := Credentials{AccessKeyID: "test", SecretAccessKey: "secret"}
	got, err := URL("GET", "http://localhost:8333/uploads/my%20cat.png", c, "us-east-1", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/uploads/my%20cat.png?") {
		t.Errorf("path not kept percent-encoded: %s", got)
	}
}

func TestRejectsBadInput(t *testing.T) {
	c := Credentials{AccessKeyID: "test", SecretAccessKey: "secret"}
	if _, err := URL("GET", "http://x/b/k", c, "us-east-1", time.Now(), 8*24*time.Hour); err == nil {
		t.Error("want error for expiry over 7 days")
	}
	if _, err := URL("GET", "http://x/b/k", Credentials{}, "us-east-1", time.Now(), time.Minute); err == nil {
		t.Error("want error for missing credentials")
	}
}
