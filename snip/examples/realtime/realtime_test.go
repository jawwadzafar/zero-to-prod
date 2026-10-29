package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestSlowSubscriberIsCutOff(t *testing.T) {
	h := NewHub()
	_, live, cancel := h.Subscribe(0)
	defer cancel()
	for i := 0; i < subBuffer+1; i++ { // one more than its buffer holds, and nobody reads
		h.Publish("x")
	}
	n := 0
	for range live { // drains the buffered events, then sees the channel closed
		n++
	}
	if n != subBuffer {
		t.Fatalf("got %d buffered events before close, want %d", n, subBuffer)
	}
	if h.Subscribers() != 0 {
		t.Fatalf("slow subscriber still registered")
	}
}

func TestHistoryIsBounded(t *testing.T) {
	h := NewHub()
	for i := 0; i < historySize+10; i++ {
		h.Publish("x")
	}
	got := h.Since(0)
	if len(got) != historySize || got[0].ID != 11 {
		t.Fatalf("history: len %d first %d, want %d events starting at 11", len(got), got[0].ID, historySize)
	}
}

// readSSE returns a function that reads the next complete event (id, data).
func readSSE(t *testing.T, r *bufio.Reader) func() (string, string) {
	return func() (id, data string) {
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("reading stream: %v", err)
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "id: "):
				id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				if data != "" {
					data += "\n"
				}
				data += strings.TrimPrefix(line, "data: ")
			case line == "" && id != "":
				return id, data
			}
		}
	}
}

func TestSSEStreamsAndResumes(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(newServer(hub).routes())
	defer srv.Close()

	hub.Publish("one")
	hub.Publish("two\nlines")
	hub.Publish("three")

	// Reconnect as a browser would, having already seen event 1.
	req, _ := http.NewRequest("GET", srv.URL+"/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	next := readSSE(t, bufio.NewReader(resp.Body))

	if id, data := next(); id != "2" || data != "two\nlines" {
		t.Fatalf("first event after resume: %s %q", id, data)
	}
	if id, _ := next(); id != "3" {
		t.Fatalf("second event after resume: %s", id)
	}
	hub.Publish("live") // and now a live one
	if id, data := next(); id != "4" || data != "live" {
		t.Fatalf("live event: %s %q", id, data)
	}
}

func TestLongPollWaitsForNews(t *testing.T) {
	hub := NewHub()
	s := newServer(hub)
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		hub.Publish("news")
	}()
	start := time.Now()
	resp, err := http.Get(srv.URL + "/longpoll?after=0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []Event
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Data != "news" {
		t.Fatalf("got %+v", got)
	}
	if time.Since(start) < 90*time.Millisecond {
		t.Fatalf("long poll answered before there was news")
	}
}

func TestLongPollTimesOutEmpty(t *testing.T) {
	s := newServer(NewHub())
	s.longPoll = 50 * time.Millisecond
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/longpoll")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []Event
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty list, got %+v", got)
	}
}

func TestWebSocketBothWays(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(newServer(hub).routes())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	// Client → server: our message becomes an event, and comes back to us.
	if err := c.Write(ctx, websocket.MessageText, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	var e Event
	if err := wsjson.Read(ctx, c, &e); err != nil {
		t.Fatal(err)
	}
	if e.Data != "ws: hi" {
		t.Fatalf("got %+v", e)
	}

	// Server → client: an event published elsewhere arrives unasked.
	hub.Publish("from the server")
	if err := wsjson.Read(ctx, c, &e); err != nil {
		t.Fatal(err)
	}
	if e.Data != "from the server" {
		t.Fatalf("got %+v", e)
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func TestCloseEndsStreams(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(newServer(hub).routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	for hub.Subscribers() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	hub.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after hub.Close")
	}
}
