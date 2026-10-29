package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

func key(t *testing.T) []byte {
	t.Helper()
	k, err := ParseSecret(NewSecret())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSignMatchesStandardWebhooksExample(t *testing.T) {
	// The test vector from the Standard Webhooks reference libraries.
	k, err := ParseSecret("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
	if err != nil {
		t.Fatal(err)
	}
	got := Sign(k, "msg_p5jXN8AQM9LWM0D4loKWxJek", time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))
	want := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestVerify(t *testing.T) {
	k, other := key(t), key(t)
	now := time.Now()
	body := []byte(`{"type":"link.created"}`)
	signed := func(keys [][]byte, ts time.Time) http.Header {
		h := http.Header{}
		SignHeaders(h, keys, "msg_1", ts, body)
		return h
	}

	cases := []struct {
		name    string
		keys    [][]byte // what the receiver knows
		h       http.Header
		body    []byte
		wantErr error
	}{
		{"valid", [][]byte{k}, signed([][]byte{k}, now), body, nil},
		{"tampered body", [][]byte{k}, signed([][]byte{k}, now), []byte(`{"type":"link.deleted"}`), ErrSignature},
		{"wrong secret", [][]byte{other}, signed([][]byte{k}, now), body, ErrSignature},
		{"too old (replay)", [][]byte{k}, signed([][]byte{k}, now.Add(-6*time.Minute)), body, ErrTimestamp},
		{"from the future", [][]byte{k}, signed([][]byte{k}, now.Add(6*time.Minute)), body, ErrTimestamp},
		{"missing headers", [][]byte{k}, http.Header{}, body, ErrMissingHeaders},
		{"rotation: sender signs with both, receiver knows new", [][]byte{other}, signed([][]byte{k, other}, now), body, nil},
		{"rotation: sender signs with new, receiver knows both", [][]byte{k, other}, signed([][]byte{other}, now), body, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Verify(c.keys, c.h, c.body, now, 5*time.Minute)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("got %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestReceiverDeduplicatesAndQueues(t *testing.T) {
	k := key(t)
	rc := NewReceiver([][]byte{k}, 10)
	srv := httptest.NewServer(rc)
	defer srv.Close()

	s := NewSender(srv.Client(), [][]byte{k})
	s.Schedule = nil // no retries needed here
	body := []byte(`{"type":"link.created"}`)
	for i := 0; i < 3; i++ { // the same message three times
		log, err := s.Deliver(context.Background(), srv.URL, "msg_same", body)
		if err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
		want := http.StatusNoContent
		if i > 0 {
			want = http.StatusOK
		}
		if log[0].Status != want {
			t.Fatalf("delivery %d: status %d, want %d", i, log[0].Status, want)
		}
	}
	if len(rc.Queue) != 1 {
		t.Fatalf("queue has %d events, want 1", len(rc.Queue))
	}
}

func TestReceiverRefusesWhenQueueFull(t *testing.T) {
	k := key(t)
	rc := NewReceiver([][]byte{k}, 1)
	srv := httptest.NewServer(rc)
	defer srv.Close()
	s := NewSender(srv.Client(), [][]byte{k})
	s.Schedule = nil

	if _, err := s.Deliver(context.Background(), srv.URL, "msg_a", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	log, err := s.Deliver(context.Background(), srv.URL, "msg_b", []byte(`{}`))
	if !errors.Is(err, ErrDeadLettered) || log[0].Status != http.StatusServiceUnavailable {
		t.Fatalf("got %v %+v, want 503 then give up", err, log)
	}
	<-rc.Queue // the worker catches up...
	if _, err := s.Deliver(context.Background(), srv.URL, "msg_b", []byte(`{}`)); err != nil {
		t.Fatalf("retry after the queue drained: %v", err) // ...and the retry is accepted
	}
}

func TestSenderRetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var waits []time.Duration
	s := NewSender(srv.Client(), [][]byte{key(t)})
	s.Schedule = []time.Duration{time.Second, time.Minute, time.Hour}
	s.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }

	log, err := s.Deliver(context.Background(), srv.URL, "msg_1", []byte(`{}`))
	if err != nil || len(log) != 3 {
		t.Fatalf("got %v after %d attempts, want success on attempt 3", err, len(log))
	}
	if len(waits) != 2 || waits[0] < time.Second || waits[0] > 1100*time.Millisecond || waits[1] < time.Minute {
		t.Fatalf("waits %v: want ~1s then ~1m (schedule plus up to 10%% jitter)", waits)
	}
}

func TestSenderStopsOnGoneAndHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	var waits []time.Duration
	s := NewSender(srv.Client(), [][]byte{key(t)})
	s.Schedule = []time.Duration{time.Second, time.Second, time.Second}
	s.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }

	log, err := s.Deliver(context.Background(), srv.URL, "msg_1", []byte(`{}`))
	if !errors.Is(err, ErrGone) || len(log) != 2 {
		t.Fatalf("got %v after %d attempts, want ErrGone after 2", err, len(log))
	}
	if waits[0] != 120*time.Second {
		t.Fatalf("waited %v, want the 120 s the receiver asked for", waits[0])
	}
}

func TestSenderDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()
	client := srv.Client()
	client.CheckRedirect = SafeClient().CheckRedirect
	s := NewSender(client, [][]byte{key(t)})
	s.Schedule = nil
	log, err := s.Deliver(context.Background(), srv.URL, "msg_1", []byte(`{}`))
	if !errors.Is(err, ErrDeadLettered) || log[0].Status != http.StatusFound {
		t.Fatalf("got %v %+v: want the 302 recorded as a failure, not followed", err, log)
	}
}

func TestSSRF(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "::1", "fd00:ec2::254", "fe80::1", "::ffff:10.0.0.5"} {
		if isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s counted as public", s)
		}
	}
	for _, s := range []string{"93.184.215.14", "2606:4700::6810:84e5"} {
		if !isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s counted as internal", s)
		}
	}

	// The dialer check catches internal IPs at connect time, whatever the URL said.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached an internal server")
	}))
	defer srv.Close()
	_, err := SafeClient().Get(srv.URL)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}

	for raw, ok := range map[string]bool{
		"https://hooks.example.com/in":   true,
		"http://hooks.example.com/in":    false,
		"https://user:pw@example.com/in": false,
		"ftp://example.com/":             false,
	} {
		if err := ValidateURL(raw); (err == nil) != ok {
			t.Errorf("ValidateURL(%q) = %v", raw, err)
		}
	}
}
