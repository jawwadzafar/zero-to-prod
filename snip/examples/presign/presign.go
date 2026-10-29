// Package presign creates S3 presigned URLs (AWS Signature Version 4, query
// string form) with the standard library only, so you can see that a
// presigned URL is nothing more than a URL plus an HMAC signature
// (chapter 6.10). Production code would use the AWS SDK's presign client;
// this works against AWS S3 and S3-compatible servers alike.
package presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxExpires is the longest validity S3 accepts for a SigV4 presigned URL.
const MaxExpires = 7 * 24 * time.Hour

// Credentials are the access key pair the signature is made with. The secret
// never appears in the URL; only the access key ID does.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

// URL returns a presigned version of rawURL (for example
// http://localhost:8333/uploads/cat.png) that allows exactly one method on
// exactly that object until now+expires. Anyone holding the result can use it.
func URL(method, rawURL string, c Credentials, region string, now time.Time, expires time.Duration) (string, error) {
	if expires <= 0 || expires > MaxExpires {
		return "", fmt.Errorf("expires must be between 1s and %s", MaxExpires)
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return "", errors.New("missing credentials")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	now = now.UTC()
	amzDate := now.Format("20060102T150405Z") // when it was signed
	day := now.Format("20060102")
	scope := day + "/" + region + "/s3/aws4_request" // what the key may sign for

	// 1. The parameters that travel in the URL itself.
	q := u.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", c.AccessKeyID+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")

	// 2. The canonical request: a fixed text form of exactly what is allowed.
	// Change any part (method, object, expiry, host) and the signature breaks.
	canonical := strings.Join([]string{
		method,
		encode(u.EscapedPath(), true),
		canonicalQuery(q),
		"host:" + u.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD", // the body isn't signed: the uploader chooses it
	}, "\n")

	// 3. The string to sign ties that request to a time and a scope.
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])

	// 4. Derive a signing key valid only for this day, region and service,
	// then sign. The server repeats steps 2–4 with its copy of the secret
	// and compares.
	key := hmacSHA256([]byte("AWS4"+c.SecretAccessKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	u.RawQuery = canonicalQuery(q) + "&X-Amz-Signature=" + sig
	return u.String(), nil
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// canonicalQuery sorts parameters by name and percent-encodes them the way
// SigV4 requires (which is stricter than url.Values.Encode: spaces become %20).
func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, encode(k, false)+"="+encode(v, false))
		}
	}
	return strings.Join(parts, "&")
}

// encode percent-encodes everything except A–Z a–z 0–9 - _ . ~ (and '/'
// when encoding a path). An already-escaped path is decoded first.
func encode(s string, path bool) string {
	if path {
		if dec, err := url.PathUnescape(s); err == nil {
			s = dec
		}
		if s == "" {
			s = "/"
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '_', c == '.', c == '~', path && c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
