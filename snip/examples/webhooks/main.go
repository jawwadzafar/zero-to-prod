// webhooks — signing, verifying, retrying and SSRF-proofing webhooks in the
// Standard Webhooks format (chapter 8.8).
//
//	go run ./examples/webhooks demo        # every scenario, end to end, in one run
//	go run ./examples/webhooks serve       # a receiver on :8095/webhooks; prints its secret
//	go run ./examples/webhooks sign -secret whsec_… -body '{"type":"link.created"}'
//	                                       # prints a signed curl command for that receiver
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func main() {
	cmd := "demo"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "demo":
		demo()
	case "serve":
		err = serve(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q (want demo, serve or sign)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "webhooks:", err)
		os.Exit(1)
	}
}

// NewMessageID makes a unique ID like msg_3f9c1a…, fixed for a message's life.
func NewMessageID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func printLog(attempts []Attempt, err error) {
	for i, a := range attempts {
		status := fmt.Sprint(a.Status)
		if a.Err != "" {
			status += " (" + a.Err + ")"
		}
		fmt.Printf("    attempt %d: %s\n", i+1, status)
	}
	if err != nil {
		fmt.Printf("    result: %v\n", err)
	}
}

func demo() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	log.SetPrefix("    receiver: ")
	ctx := context.Background()

	oldSecret, newSecret := NewSecret(), NewSecret()
	oldKey, _ := ParseSecret(oldSecret)
	newKey, _ := ParseSecret(newSecret)

	rc := NewReceiver([][]byte{oldKey}, 100)
	recv := httptest.NewServer(rc)
	defer recv.Close()
	url := recv.URL + "/webhooks"

	// The demo receiver is on 127.0.0.1, so this sender uses a plain client.
	// Real senders use SafeClient (step 7 shows why).
	s := NewSender(http.DefaultClient, [][]byte{oldKey})
	s.Schedule = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

	body := []byte(`{"type":"link.created","timestamp":"2026-10-29T12:00:00Z","data":{"slug":"launch"}}`)
	id := NewMessageID()

	fmt.Println("1. A signed event arrives: verified, queued, answered at once.")
	printLog(s.Deliver(ctx, url, id, body))

	fmt.Println("2. The same event again (a retry after a lost response): 200, not processed twice.")
	printLog(s.Deliver(ctx, url, id, body))

	fmt.Println("3. Someone changes the body after signing: rejected.")
	fmt.Printf("    status: %d\n", post(url, oldKey, NewMessageID(), time.Now(), body, []byte(`{"type":"link.created","data":{"slug":"evil"}}`)))

	fmt.Println("4. A captured request replayed 10 minutes later: rejected by the timestamp check.")
	fmt.Printf("    status: %d\n", post(url, oldKey, NewMessageID(), time.Now().Add(-10*time.Minute), body, body))

	fmt.Println("5. Rotating the secret: the sender signs with old AND new; the receiver moves to new only.")
	s.Keys = [][]byte{oldKey, newKey}
	rc.Keys = [][]byte{newKey}
	printLog(s.Deliver(ctx, url, NewMessageID(), body))

	fmt.Println("6. A receiver that fails twice, then recovers: retries with backoff.")
	var calls atomic.Int32
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "database down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer flaky.Close()
	printLog(s.Deliver(ctx, flaky.URL, NewMessageID(), body))

	fmt.Println("   ...and one that never recovers: dead-lettered after the last retry.")
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()
	printLog(s.Deliver(ctx, broken.URL, NewMessageID(), body))

	fmt.Println("7. SSRF: a user sets their webhook URL to an internal address.")
	for _, u := range []string{"http://169.254.169.254/latest/meta-data/", "https://169.254.169.254/latest/meta-data/", "https://127.0.0.1:6379/", "https://[::ffff:10.0.0.5]/"} {
		if err := ValidateURL(u); err != nil {
			fmt.Printf("    %-42s refused when saved: %v\n", u, err)
			continue
		}
		_, err := SafeClient().Post(u, "application/json", strings.NewReader("{}"))
		fmt.Printf("    %-42s refused when sending: %v\n", u, errors.Unwrap(errors.Unwrap(err)))
	}

	fmt.Printf("\nThe receiver's queue holds %d events for its background worker, one per unique message.\n", len(rc.Queue))
}

// post sends one hand-made request: signed over signedBody, carrying sentBody.
func post(url string, key []byte, id string, ts time.Time, signedBody, sentBody []byte) int {
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(sentBody)))
	SignHeaders(req.Header, [][]byte{key}, id, ts, signedBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8095", "address to listen on")
	secret := fs.String("secret", os.Getenv("WEBHOOK_SECRET"), "whsec_… secret (default: $WEBHOOK_SECRET, or a new one)")
	_ = fs.Parse(args)
	if *secret == "" {
		*secret = NewSecret()
	}
	key, err := ParseSecret(*secret)
	if err != nil {
		return err
	}
	rc := NewReceiver([][]byte{key}, 1000)
	go func() { // the background worker: where the real (slow) work would go
		for m := range rc.Queue {
			log.Printf("processing %s: %s", m.ID, m.Body)
		}
	}()
	mux := http.NewServeMux()
	mux.Handle("POST /webhooks", rc)
	log.Printf("receiving on http://localhost%s/webhooks", *addr)
	log.Printf("secret: %s", *secret)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return srv.ListenAndServe()
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	secret := fs.String("secret", os.Getenv("WEBHOOK_SECRET"), "whsec_… secret")
	body := fs.String("body", `{"type":"ping","data":{}}`, "JSON body to sign")
	id := fs.String("id", NewMessageID(), "webhook-id (reuse one to test de-duplication)")
	age := fs.Duration("age", 0, "pretend the message was signed this long ago (test replay protection)")
	url := fs.String("url", "http://localhost:8095/webhooks", "where to send it")
	_ = fs.Parse(args)
	key, err := ParseSecret(*secret)
	if err != nil {
		return err
	}
	h := http.Header{}
	SignHeaders(h, [][]byte{key}, *id, time.Now().Add(-*age), []byte(*body))
	fmt.Printf("curl -si %s \\\n  -H 'content-type: application/json' \\\n", *url)
	for _, k := range []string{"webhook-id", "webhook-timestamp", "webhook-signature"} {
		fmt.Printf("  -H '%s: %s' \\\n", k, h.Get(k))
	}
	fmt.Printf("  --data-raw '%s'\n", *body)
	return nil
}
