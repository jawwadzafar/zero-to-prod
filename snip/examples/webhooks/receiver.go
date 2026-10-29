package main

import (
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Received is one verified webhook, waiting to be processed.
type Received struct {
	ID   string
	Body []byte
}

// Receiver is an HTTP handler for incoming webhooks. It does the minimum
// before answering: verify, de-duplicate, put the event on a queue. The
// slow work happens later, in whatever reads Queue.
type Receiver struct {
	Keys      [][]byte      // current secret, plus the old one during a rotation
	Tolerance time.Duration // how far webhook-timestamp may be from now (5 min is common)
	Queue     chan Received // processed in the background
	Now       func() time.Time

	mu   sync.Mutex
	seen map[string]time.Time // webhook-id → when first accepted
}

func NewReceiver(keys [][]byte, queueSize int) *Receiver {
	return &Receiver{
		Keys:      keys,
		Tolerance: 5 * time.Minute,
		Queue:     make(chan Received, queueSize),
		Now:       time.Now,
		seen:      map[string]time.Time{},
	}
}

func (rc *Receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// Read the RAW bytes. The signature covers them exactly; parsing JSON and
	// re-encoding it would change spacing or key order and break the check.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := Verify(rc.Keys, r.Header, body, rc.Now(), rc.Tolerance); err != nil {
		// 401 with no detail: don't tell an attacker which check failed.
		log.Printf("rejected webhook %q: %v", r.Header.Get("webhook-id"), err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	id := r.Header.Get("webhook-id")
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, dup := rc.seen[id]; dup {
		// Already have it: a retry of something we accepted. Say "fine" so
		// the sender stops, and don't do the work twice.
		w.WriteHeader(http.StatusOK)
		return
	}
	select {
	case rc.Queue <- Received{ID: id, Body: body}:
		rc.seen[id] = rc.Now()
		w.WriteHeader(http.StatusNoContent) // accepted; processing happens later
	default:
		// Queue full: refuse, and the sender will retry later. Not marking it
		// seen means the retry is accepted once there's room.
		http.Error(w, "busy, retry later", http.StatusServiceUnavailable)
	}
}

// Forget drops de-duplication records older than maxAge. Senders stop
// retrying after a few days, so IDs need not be kept forever.
func (rc *Receiver) Forget(maxAge time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for id, at := range rc.seen {
		if rc.Now().Sub(at) > maxAge {
			delete(rc.seen, id)
		}
	}
}
