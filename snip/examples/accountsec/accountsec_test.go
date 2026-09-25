package accountsec

import (
	"context"
	"crypto/sha1" //nolint:gosec // test mirrors the API's hashing
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResetTokensAreSingleUseAndExpire(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	r := NewResets(15 * time.Minute)
	r.Now = func() time.Time { return now }

	token, err := r.Issue("ada")
	if err != nil {
		t.Fatal(err)
	}
	for h := range r.byHash {
		if strings.Contains(h, token) || h == token {
			t.Fatal("the token itself must not be stored")
		}
	}
	if user, err := r.Redeem(token); err != nil || user != "ada" {
		t.Fatalf("first redeem: %q, %v", user, err)
	}
	if _, err := r.Redeem(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("a token must work only once")
	}

	late, _ := r.Issue("ada")
	now = now.Add(16 * time.Minute)
	if _, err := r.Redeem(late); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("an expired token must be refused")
	}
}

func TestNewTokenCancelsOlderOnes(t *testing.T) {
	r := NewResets(time.Hour)
	first, _ := r.Issue("ada")
	second, _ := r.Issue("ada")
	if _, err := r.Redeem(first); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("issuing a new link should cancel the old one")
	}
	if _, err := r.Redeem(second); err != nil {
		t.Fatal(err)
	}
}

func TestBreachCountSendsOnlyAPrefix(t *testing.T) {
	sum := sha1.Sum([]byte("password123")) //nolint:gosec // test
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		fmt.Fprintf(w, "0018A45C4D1DEF81644B54AB7F969B88D65:10\r\n%s:251682\r\nFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:0\r\n", full[5:])
	}))
	defer srv.Close()

	n, err := BreachCount(context.Background(), srv.Client(), srv.URL+"/range/", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if n != 251682 {
		t.Fatalf("count = %d", n)
	}
	if seenPath != "/range/"+full[:5] {
		t.Fatalf("server saw %q: it must only ever see the 5-character prefix", seenPath)
	}
	n, _ = BreachCount(context.Background(), srv.Client(), srv.URL+"/range/", "a much longer passphrase nobody uses")
	if n != 0 {
		t.Fatalf("an unseen password should count 0, got %d", n)
	}
}
