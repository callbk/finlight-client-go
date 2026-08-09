package finlight

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fastWSOptions keep reconnect tests quick.
var fastWSOptions = WebSocketOptions{
	BaseReconnectDelay: 10 * time.Millisecond,
	MaxReconnectDelay:  50 * time.Millisecond,
}

// newWSTestServer runs handler once per WebSocket connection.
func newWSTestServer(t *testing.T, handler func(r *http.Request, conn *websocket.Conn)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		handler(r, conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func sendJSON(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Logf("server write failed: %v", err)
	}
}

func readJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("server read failed: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

// waitClose drains the connection until the peer completes the close
// handshake, so client-side Close calls don't run into their 5s wait.
func waitClose(conn *websocket.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func testArticleMsg(link string) map[string]any {
	return map[string]any{
		"action": "sendArticle",
		"data": map[string]any{
			"link":        link,
			"title":       "Title " + link,
			"publishDate": "2024-01-01T00:00:00Z",
			"source":      "example.com",
			"language":    "en",
		},
	}
}

func TestStreamHandshakeDedupAndPreempt(t *testing.T) {
	var handshake map[string]any
	var headers http.Header
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		headers = r.Header.Clone()
		handshake = readJSON(t, conn)
		sendJSON(t, conn, map[string]any{"action": "admit", "leaseId": "lease-1", "clientNonce": handshake["clientNonce"]})
		sendJSON(t, conn, testArticleMsg("https://example.com/a"))
		sendJSON(t, conn, testArticleMsg("https://example.com/a")) // duplicate
		sendJSON(t, conn, testArticleMsg("https://example.com/b"))
		sendJSON(t, conn, map[string]any{"action": "preempted", "reason": "test over"})
		waitClose(conn)
	})

	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, WebSocketOptions{Takeover: true})
	var links []string
	for article := range ws.Stream(context.Background(), GetArticlesWebSocketParams{Query: "nvidia"}) {
		links = append(links, article.Link)
	}
	if err := ws.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}

	if got := headers.Get("x-api-key"); got != "test-key" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := headers.Get("x-client-version"); !strings.HasPrefix(got, "go/finlight-client-go@") {
		t.Errorf("x-client-version = %q", got)
	}
	if got := headers.Get("x-takeover"); got != "true" {
		t.Errorf("x-takeover = %q", got)
	}
	if handshake["query"] != "nvidia" {
		t.Errorf("handshake payload = %v", handshake)
	}
	if nonce, _ := handshake["clientNonce"].(string); len(nonce) != 36 {
		t.Errorf("clientNonce = %v", handshake["clientNonce"])
	}
	want := []string{"https://example.com/a", "https://example.com/b"}
	if len(links) != len(want) || links[0] != want[0] || links[1] != want[1] {
		t.Errorf("links = %v, want %v (duplicate must be suppressed)", links, want)
	}
}

func TestRawStreamDoesNotDedup(t *testing.T) {
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn)
		sendJSON(t, conn, testArticleMsg("https://example.com/a"))
		sendJSON(t, conn, testArticleMsg("https://example.com/a"))
		sendJSON(t, conn, map[string]any{"action": "preempted"})
		waitClose(conn)
	})

	// The raw client appends /raw to the configured URL; strip nothing here —
	// the test server accepts any path.
	ws := NewRawWebSocketClient(Config{APIKey: "test-key", WssURL: url}, WebSocketOptions{})
	var count int
	for range ws.Stream(context.Background(), GetRawArticlesWebSocketParams{}) {
		count++
	}
	if err := ws.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if count != 2 {
		t.Errorf("articles = %d, want 2 (raw stream must not dedup)", count)
	}
}

func TestStreamBlockedCloseCode(t *testing.T) {
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn)
		_ = conn.Close(websocket.StatusPolicyViolation, "blocked")
	})

	var closeCode atomic.Int64
	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, WebSocketOptions{
		OnClose: func(code int, reason string) { closeCode.Store(int64(code)) },
	})
	for range ws.Stream(context.Background(), GetArticlesWebSocketParams{}) {
		t.Fatal("no articles expected")
	}
	if err := ws.Err(); !errors.Is(err, ErrBlocked) {
		t.Fatalf("Err() = %v, want ErrBlocked", err)
	}
	if got := closeCode.Load(); got != 1008 {
		t.Errorf("OnClose code = %d, want 1008", got)
	}
}

func TestStreamReconnectsAfterServerClose(t *testing.T) {
	var conns atomic.Int32
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn)
		if conns.Add(1) == 1 {
			_ = conn.Close(websocket.StatusNormalClosure, "bye") // trigger reconnect
			return
		}
		sendJSON(t, conn, testArticleMsg("https://example.com/after-reconnect"))
		sendJSON(t, conn, map[string]any{"action": "preempted"})
		waitClose(conn)
	})

	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, fastWSOptions)
	var links []string
	for article := range ws.Stream(context.Background(), GetArticlesWebSocketParams{}) {
		links = append(links, article.Link)
	}
	if err := ws.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if got := conns.Load(); got != 2 {
		t.Errorf("connections = %d, want 2", got)
	}
	if len(links) != 1 || links[0] != "https://example.com/after-reconnect" {
		t.Errorf("links = %v", links)
	}
}

func TestStreamConsumerBreakStopsStream(t *testing.T) {
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn)
		for i := 0; i < 5; i++ {
			sendJSON(t, conn, testArticleMsg("https://example.com/"+string(rune('a'+i))))
		}
		// Keep the connection open; the client must close it on break.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, _ = conn.Read(ctx)
	})

	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, WebSocketOptions{})
	var count int
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ws.Stream(context.Background(), GetArticlesWebSocketParams{}) {
			count++
			break
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop after consumer break")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if err := ws.Err(); err != nil {
		t.Errorf("Err() = %v, want nil after consumer break", err)
	}
}

func TestStreamContextCancel(t *testing.T) {
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, _ = conn.Read(ctx) // idle until client disconnects
	})

	ctx, cancel := context.WithCancel(context.Background())
	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, WebSocketOptions{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ws.Stream(ctx, GetArticlesWebSocketParams{}) {
		}
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop after context cancel")
	}
	if err := ws.Err(); err != nil {
		t.Errorf("Err() = %v, want nil after context cancel", err)
	}
}

func TestStreamSendsApplicationPing(t *testing.T) {
	gotPing := make(chan map[string]any, 1)
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		readJSON(t, conn) // handshake
		msg := readJSON(t, conn)
		select {
		case gotPing <- msg:
		default:
		}
		sendJSON(t, conn, map[string]any{"action": "pong", "t": msg["t"]})
		sendJSON(t, conn, map[string]any{"action": "preempted"})
		waitClose(conn)
	})

	opts := fastWSOptions
	opts.PingInterval = 30 * time.Millisecond
	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, opts)
	for range ws.Stream(context.Background(), GetArticlesWebSocketParams{}) {
	}
	select {
	case msg := <-gotPing:
		if msg["action"] != "ping" {
			t.Errorf("expected ping, got %v", msg)
		}
		if _, ok := msg["t"].(float64); !ok {
			t.Errorf("ping without timestamp: %v", msg)
		}
	default:
		t.Fatal("no ping received")
	}
}

func TestStreamAdminKickDelaysReconnect(t *testing.T) {
	const retryAfterMs = 300
	var conns atomic.Int32
	var firstClose atomic.Int64
	var secondDial atomic.Int64
	url := newWSTestServer(t, func(r *http.Request, conn *websocket.Conn) {
		n := conns.Add(1)
		if n == 1 {
			readJSON(t, conn)
			sendJSON(t, conn, map[string]any{"action": "admin_kick", "retryAfter": retryAfterMs})
			waitClose(conn)
			firstClose.Store(time.Now().UnixMilli())
			return
		}
		secondDial.Store(time.Now().UnixMilli())
		readJSON(t, conn)
		sendJSON(t, conn, map[string]any{"action": "preempted"})
		waitClose(conn)
	})

	ws := NewWebSocketClient(Config{APIKey: "test-key", WssURL: url}, fastWSOptions)
	for range ws.Stream(context.Background(), GetArticlesWebSocketParams{}) {
	}
	if err := ws.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if got := conns.Load(); got != 2 {
		t.Fatalf("connections = %d, want 2", got)
	}
	if elapsed := secondDial.Load() - firstClose.Load(); elapsed < retryAfterMs-100 {
		t.Errorf("reconnected after %dms, want >= ~%dms (admin_kick retryAfter)", elapsed, retryAfterMs)
	}
}
