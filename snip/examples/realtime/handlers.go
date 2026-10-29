package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type server struct {
	hub       *Hub
	publish   func(ctx context.Context, data string) (Event, error)
	heartbeat time.Duration // how often idle streams get a keep-alive
	longPoll  time.Duration // how long a long-poll request waits for news
}

func newServer(hub *Hub) *server {
	return &server{
		hub:       hub,
		publish:   func(_ context.Context, data string) (Event, error) { return hub.Publish(data), nil },
		heartbeat: 15 * time.Second,
		longPoll:  25 * time.Second,
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /poll", s.poll)
	mux.HandleFunc("GET /longpoll", s.longpoll)
	mux.HandleFunc("GET /events", s.sse)
	mux.HandleFunc("GET /ws", s.ws)
	mux.HandleFunc("POST /publish", s.publishHandler)
	return mux
}

// afterParam reads ?after=N. Missing or bad means 0: send everything still remembered.
func afterParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// POST /publish — the body becomes an event.
func (s *server) publishHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
	if err != nil || len(body) == 0 {
		http.Error(w, "send a non-empty body", http.StatusBadRequest)
		return
	}
	e, err := s.publish(r.Context(), strings.TrimSpace(string(body)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, e)
}

// GET /poll?after=N — short polling: answer at once, even if there's nothing.
func (s *server) poll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.hub.Since(afterParam(r, "after")))
}

// GET /longpoll?after=N — long polling: if there's nothing new yet, hold the
// request open until there is, or until s.longPoll passes (then send []).
func (s *server) longpoll(w http.ResponseWriter, r *http.Request) {
	backlog, live, cancel := s.hub.Subscribe(afterParam(r, "after"))
	defer cancel()
	if len(backlog) > 0 {
		writeJSON(w, backlog)
		return
	}
	timer := time.NewTimer(s.longPoll)
	defer timer.Stop()
	select {
	case e, ok := <-live:
		if ok {
			writeJSON(w, []Event{e})
			return
		}
	case <-timer.C:
	case <-r.Context().Done():
		return
	}
	writeJSON(w, []Event{}) // nothing new: the client asks again straight away
}

// GET /events — Server-Sent Events: one response that never ends.
func (s *server) sse(w http.ResponseWriter, r *http.Request) {
	// A reconnecting browser sends the last ID it saw in this header.
	// (?after=N does the same for curl.)
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if after == 0 {
		after = afterParam(r, "after")
	}

	rc := http.NewResponseController(w)
	// The server's WriteTimeout is meant for normal responses. A stream lives
	// for hours, so lift the deadline for this one response.
	_ = rc.SetWriteDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // ask nginx not to buffer this response

	backlog, live, cancel := s.hub.Subscribe(after)
	defer cancel()

	fmt.Fprint(w, "retry: 3000\n\n") // if the connection drops, reconnect after 3 s
	for _, e := range backlog {
		writeSSE(w, e)
	}
	if err := rc.Flush(); err != nil { // push the bytes out now, not when a buffer fills
		return
	}

	beat := time.NewTicker(s.heartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done(): // the client went away
			return
		case e, ok := <-live:
			if !ok { // the hub cut us off (too slow, or shutting down); the client reconnects
				return
			}
			writeSSE(w, e)
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n") // a comment line: ignored by clients, keeps proxies from timing out
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

// writeSSE writes one event in the text/event-stream format. A newline in
// the data would end the field, so each line gets its own "data:".
func writeSSE(w io.Writer, e Event) {
	fmt.Fprintf(w, "id: %d\n", e.ID)
	for _, line := range strings.Split(e.Data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n") // a blank line ends the event
}

// GET /ws — a WebSocket: both sides can send at any time.
func (s *server) ws(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{}) // same reason as SSE: this connection outlives the server's timeouts
	_ = rc.SetWriteDeadline(time.Time{})

	// Accept answers the upgrade handshake (101 Switching Protocols). By
	// default it refuses pages from other origins, which stops another site
	// opening a socket with your users' cookies.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has already written an error response
	}
	defer c.CloseNow()

	ctx, stop := context.WithCancel(r.Context())
	defer stop()

	backlog, live, cancel := s.hub.Subscribe(afterParam(r, "after"))
	defer cancel()

	// One goroutine reads: messages from the browser become events. Reading
	// is also what processes the peer's pongs and close frames.
	go func() {
		defer stop()
		for {
			_, msg, err := c.Read(ctx)
			if err != nil {
				return
			}
			_, _ = s.publish(ctx, "ws: "+string(msg))
		}
	}()

	send := func(e Event) error {
		wctx, done := context.WithTimeout(ctx, 5*time.Second) // a stuck client can't hold us forever
		defer done()
		return wsjson.Write(wctx, c, e)
	}
	for _, e := range backlog {
		if send(e) != nil {
			return
		}
	}

	beat := time.NewTicker(s.heartbeat)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-live:
			if !ok {
				c.Close(websocket.StatusTryAgainLater, "too slow or shutting down; reconnect")
				return
			}
			if send(e) != nil {
				return
			}
		case <-beat.C:
			// A ping frame; the browser answers with a pong automatically.
			// No pong within 5 s means the connection is dead: give up.
			pctx, done := context.WithTimeout(ctx, 5*time.Second)
			err := c.Ping(pctx)
			done()
			if err != nil {
				return
			}
		}
	}
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

const indexHTML = `<!doctype html>
<meta charset="utf-8">
<title>realtime demo</title>
<style>body{font:15px system-ui;margin:2rem;max-width:60rem} .cols{display:flex;gap:2rem} ul{font:13px monospace;height:20rem;overflow:auto;border:1px solid #ccc;padding:.5rem 1.5rem}</style>
<h1>Polling, SSE and WebSockets</h1>
<p>Open this page in two tabs. Events come from the server's ticker, from
<code>curl -X POST localhost:8090/publish -d hello</code>, and from the box below.</p>
<div class="cols">
 <div><h2>Server-Sent Events</h2><p id="sse-state">connecting…</p><ul id="sse"></ul></div>
 <div><h2>WebSocket</h2><p id="ws-state">connecting…</p>
  <form id="f"><input id="msg" placeholder="say something" autocomplete="off"> <button>send</button></form>
  <ul id="ws"></ul></div>
</div>
<script>
function add(id, text) {
  const li = document.createElement('li'); li.textContent = text;
  const ul = document.getElementById(id); ul.prepend(li);
}
// SSE: the browser reconnects by itself and sends Last-Event-ID.
const es = new EventSource('/events');
es.onopen = () => document.getElementById('sse-state').textContent = 'connected';
es.onerror = () => document.getElementById('sse-state').textContent = 'reconnecting…';
es.onmessage = (m) => add('sse', m.lastEventId + ': ' + m.data);

// WebSocket: reconnecting is our job, so remember the last ID ourselves.
let lastId = 0, sock;
function connect() {
  sock = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws?after=' + lastId);
  sock.onopen = () => document.getElementById('ws-state').textContent = 'connected';
  sock.onmessage = (m) => { const e = JSON.parse(m.data); lastId = e.id; add('ws', e.id + ': ' + e.data); };
  sock.onclose = () => {
    document.getElementById('ws-state').textContent = 'reconnecting…';
    setTimeout(connect, 1000 + Math.random() * 2000); // jitter, so thousands of tabs don't return at once
  };
}
connect();
document.getElementById('f').onsubmit = (ev) => {
  ev.preventDefault();
  const box = document.getElementById('msg');
  if (box.value && sock.readyState === WebSocket.OPEN) sock.send(box.value);
  box.value = '';
};
</script>`
