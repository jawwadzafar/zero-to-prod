package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Attempt is one row of the delivery log: what users see on a "webhook
// deliveries" page when they ask "did you send it?".
type Attempt struct {
	At       time.Time
	Status   int // 0 if no HTTP response at all
	Err      string
	Duration time.Duration
}

var (
	ErrGone         = errors.New("endpoint answered 410 Gone: disable it")
	ErrDeadLettered = errors.New("every retry failed: moved to the dead-letter list")
)

// Sender delivers signed webhooks with retries.
type Sender struct {
	Client   *http.Client
	Keys     [][]byte        // usually one; two while rotating
	Schedule []time.Duration // wait before each retry; len(Schedule)+1 attempts in all
	Sleep    func(context.Context, time.Duration) error
	Now      func() time.Time
}

// DefaultSchedule is the example from the Standard Webhooks spec: retries
// spread over about three days, so a receiver that is down for a weekend
// still gets its events.
var DefaultSchedule = []time.Duration{
	5 * time.Second, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour,
	5 * time.Hour, 10 * time.Hour, 14 * time.Hour, 20 * time.Hour, 24 * time.Hour,
}

func NewSender(client *http.Client, keys [][]byte) *Sender {
	return &Sender{Client: client, Keys: keys, Schedule: DefaultSchedule, Sleep: sleepCtx, Now: time.Now}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Deliver sends one message until it succeeds, the endpoint is gone, or the
// schedule runs out. The ID stays the same on every attempt (so the receiver
// can de-duplicate); the timestamp and signature are fresh each time.
func (s *Sender) Deliver(ctx context.Context, url, id string, body []byte) ([]Attempt, error) {
	var log []Attempt
	for attempt := 0; ; attempt++ {
		a, retryAfter := s.try(ctx, url, id, body)
		log = append(log, a)
		switch {
		case a.Status >= 200 && a.Status < 300:
			return log, nil
		case a.Status == http.StatusGone:
			return log, ErrGone
		}
		if attempt >= len(s.Schedule) {
			return log, ErrDeadLettered
		}
		wait := s.Schedule[attempt]
		wait += time.Duration(rand.Int64N(int64(wait)/10 + 1)) // up to 10% jitter: retries from many senders don't line up
		if retryAfter > wait {
			wait = retryAfter // the receiver asked us to back off longer
		}
		if err := s.Sleep(ctx, wait); err != nil {
			return log, err
		}
	}
}

func (s *Sender) try(ctx context.Context, url, id string, body []byte) (Attempt, time.Duration) {
	start := s.Now()
	a := Attempt{At: start}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second) // never wait forever for someone else's server
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		a.Err = err.Error()
		return a, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "webhooks-demo/1.0")
	SignHeaders(req.Header, s.Keys, id, start, body)

	resp, err := s.Client.Do(req)
	a.Duration = time.Since(start)
	if err != nil {
		a.Err = err.Error()
		return a, 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // read a little, so the connection can be reused
	a.Status = resp.StatusCode
	if a.Status >= 300 && a.Status < 400 {
		a.Err = fmt.Sprintf("redirect to %q not followed: update the endpoint URL", resp.Header.Get("Location"))
	}
	var retryAfter time.Duration
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
		retryAfter = time.Duration(secs) * time.Second
	}
	return a, retryAfter
}
