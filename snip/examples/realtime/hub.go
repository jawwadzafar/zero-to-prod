package main

import "sync"

// Event is one thing that happened. IDs only ever go up, so a client that
// reconnects can say "I have everything up to 41" and get 42 onwards.
type Event struct {
	ID   int64  `json:"id"`
	Data string `json:"data"`
}

const (
	historySize = 100 // recent events kept for clients that reconnect
	subBuffer   = 16  // events a subscriber may fall behind before it is cut off
)

// Hub fans events out to every connected client on this one server.
//
// Each subscriber gets its own small buffered channel. Publishing never waits
// for a slow client: if a subscriber's buffer is full, the hub closes its
// channel (the handler then ends the connection) and moves on. The client
// reconnects and catches up from history. That is the backpressure policy:
// one slow phone must not stall everyone else.
type Hub struct {
	mu      sync.Mutex
	nextID  int64
	history []Event // oldest first, at most historySize
	subs    map[chan Event]struct{}
	closed  bool
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

// Publish gives data the next ID and sends it to every subscriber.
func (h *Hub) Publish(data string) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	e := Event{ID: h.nextID, Data: data}
	h.deliverLocked(e)
	return e
}

// Deliver sends an event whose ID was assigned elsewhere (by Redis, when
// several servers share one stream of events).
func (h *Hub) Deliver(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e.ID > h.nextID {
		h.nextID = e.ID
	}
	h.deliverLocked(e)
}

func (h *Hub) deliverLocked(e Event) {
	if h.closed {
		return
	}
	h.history = append(h.history, e)
	if len(h.history) > historySize {
		h.history = h.history[len(h.history)-historySize:]
	}
	for ch := range h.subs {
		select {
		case ch <- e:
		default: // buffer full: this client is too slow; cut it off rather than block
			delete(h.subs, ch)
			close(ch)
		}
	}
}

// Subscribe returns the remembered events after `after`, and a channel of
// live events from now on. Both are taken under one lock, so no event can
// slip between "catch up" and "follow live". Call cancel when done.
func (h *Hub) Subscribe(after int64) (backlog []Event, live <-chan Event, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.history {
		if e.ID > after {
			backlog = append(backlog, e)
		}
	}
	ch := make(chan Event, subBuffer)
	if h.closed {
		close(ch)
		return backlog, ch, func() {}
	}
	h.subs[ch] = struct{}{}
	return backlog, ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
}

// Since returns the remembered events after `after` (for short polling).
func (h *Hub) Since(after int64) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Event{}
	for _, e := range h.history {
		if e.ID > after {
			out = append(out, e)
		}
	}
	return out
}

// Subscribers reports how many clients are connected right now.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Close ends every subscription. Used on shutdown, so long-lived streams
// finish instead of holding the server open until it is killed.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		close(ch)
	}
	h.subs = map[chan Event]struct{}{}
}
