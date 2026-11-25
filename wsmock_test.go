package finlight

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wsMock is an in-process WebSocket server for stream tests.
type wsMock struct {
	srv *httptest.Server

	mu       sync.Mutex
	script   []dialReply // answers for the next dials; empty: accept
	autoPong atomic.Bool
	dials    chan *dialInfo
	count    atomic.Int32
	open     atomic.Int32
}

type dialReply struct {
	status int
	body   string
}

type dialInfo struct {
	path     string
	rawQuery string
	header   http.Header
	conn     *serverConn // nil for a refused dial
}

type serverConn struct {
	c      *websocket.Conn
	in     chan json.RawMessage // frames from the client
	closed chan struct{}
	once   sync.Once
}

func newWSMock(t testing.TB) *wsMock {
	m := &wsMock{dials: make(chan *dialInfo, 64)}
	m.autoPong.Store(true)
	m.srv = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *wsMock) URL() string { return "ws" + strings.TrimPrefix(m.srv.URL, "http") }

func (m *wsMock) handle(w http.ResponseWriter, r *http.Request) {
	m.count.Add(1)
	m.mu.Lock()
	reply := dialReply{status: 101}
	if len(m.script) > 0 {
		reply = m.script[0]
		m.script = m.script[1:]
	}
	m.mu.Unlock()
	info := &dialInfo{path: r.URL.Path, rawQuery: r.URL.RawQuery, header: r.Header.Clone()}
	if reply.status != 101 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
		m.dials <- info
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(64 << 20)
	sc := &serverConn{c: c, in: make(chan json.RawMessage, 1024), closed: make(chan struct{})}
	info.conn = sc
	m.open.Add(1)
	defer m.open.Add(-1)
	m.dials <- info
	for {
		_, data, err := c.Read(context.Background())
		if err != nil {
			sc.markClosed()
			return
		}
		var f map[string]any
		if json.Unmarshal(data, &f) == nil && f["action"] == "ping" && m.autoPong.Load() {
			pong, _ := json.Marshal(map[string]any{
				"event": "connection.pong", "payloadVersion": 3,
				"data": map[string]any{"t": f["t"], "serverNow": 1795467845123},
			})
			_ = c.Write(context.Background(), websocket.MessageText, pong)
		}
		sc.in <- append(json.RawMessage(nil), data...)
	}
}

func (s *serverConn) markClosed() { s.once.Do(func() { close(s.closed) }) }

func (s *serverConn) isOpen() bool {
	select {
	case <-s.closed:
		return false
	default:
		return true
	}
}

func (s *serverConn) send(t testing.TB, frame any) {
	t.Helper()
	data, ok := frame.(json.RawMessage)
	if !ok {
		data = mustJSON(t, frame)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("server send: %v", err)
	}
}

func (s *serverConn) close() {
	_ = s.c.Close(websocket.StatusNormalClosure, "")
	s.markClosed()
}

// next returns the next frame the client sent.
func (s *serverConn) next(t testing.TB, what string) map[string]any {
	t.Helper()
	select {
	case data := <-s.in:
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: client frame is not JSON: %s", what, data)
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	return nil
}

// nextNonPing skips keep-alive pings.
func (s *serverConn) nextNonPing(t testing.TB, what string) map[string]any {
	t.Helper()
	for {
		m := s.next(t, what)
		if m["action"] != "ping" {
			return m
		}
	}
}

// peekPing consumes a queued ping, if one is queued.
func (s *serverConn) peekPing() bool {
	select {
	case data := <-s.in:
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["action"] == "ping" {
			return true
		}
		// Not a ping: put it back in front by re-queuing everything.
		rest := []json.RawMessage{data}
		for {
			select {
			case d := <-s.in:
				rest = append(rest, d)
				continue
			default:
			}
			break
		}
		for _, d := range rest {
			s.in <- d
		}
		return false
	default:
		return false
	}
}

func (m *wsMock) push(replies ...dialReply) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.script = append(m.script, replies...)
}

func (m *wsMock) nextDial(t testing.TB, what string) *dialInfo {
	t.Helper()
	select {
	case d := <-m.dials:
		return d
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	return nil
}
