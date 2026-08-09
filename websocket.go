package finlight

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// WebSocketOptions tune the streaming clients. Zero values fall back to the
// documented defaults, which match the TypeScript and Python clients.
type WebSocketOptions struct {
	PingInterval       time.Duration // application-level ping cadence, default 25s
	PongTimeout        time.Duration // force reconnect when no pong arrives, default 60s
	BaseReconnectDelay time.Duration // first reconnect backoff, default 500ms
	MaxReconnectDelay  time.Duration // backoff cap, default 10s
	ConnectionLifetime time.Duration // proactive rotation, default 115min (under the 2h server cap)
	Takeover           bool          // take over an existing connection for the same key
	OnClose            func(code int, reason string)
}

func (o WebSocketOptions) withDefaults() WebSocketOptions {
	if o.PingInterval == 0 {
		o.PingInterval = 25 * time.Second
	}
	if o.PongTimeout == 0 {
		o.PongTimeout = 60 * time.Second
	}
	if o.BaseReconnectDelay == 0 {
		o.BaseReconnectDelay = 500 * time.Millisecond
	}
	if o.MaxReconnectDelay == 0 {
		o.MaxReconnectDelay = 10 * time.Second
	}
	if o.ConnectionLifetime == 0 {
		o.ConnectionLifetime = 115 * time.Minute
	}
	return o
}

// Close codes used by the finlight WebSocket protocol.
const (
	closeProactiveRotation websocket.StatusCode = 4000
	closeRateLimited       websocket.StatusCode = 4001
	closeUserBlocked       websocket.StatusCode = 4002
	closeAdminKick         websocket.StatusCode = 4003
)

const (
	recentArticleCacheSize = 10
	watchdogInterval       = 5 * time.Second
	dialRateLimitBackoff   = 60 * time.Second
	errorRateLimitBackoff  = 60 * time.Second
	errorBlockedBackoff    = time.Hour
	defaultAdminKickRetry  = 15 * time.Minute
	maxArticleMessageSize  = 16 << 20
)

// wsMessage is the envelope for every server message.
type wsMessage struct {
	Action      string          `json:"action"`
	T           int64           `json:"t"`
	LeaseID     string          `json:"leaseId"`
	ClientNonce string          `json:"clientNonce"`
	Reason      string          `json:"reason"`
	NewLeaseID  string          `json:"newLeaseId"`
	RetryAfter  int64           `json:"retryAfter"` // milliseconds
	Data        json.RawMessage `json:"data"`
	Error       json.RawMessage `json:"error"`
}

// wsCore implements the shared streaming protocol for both article types:
// reconnect loop with exponential backoff, application-level ping/pong with
// watchdog, proactive connection rotation, and optional duplicate suppression.
type wsCore[T any] struct {
	cfg      Config
	opts     WebSocketOptions
	url      string
	log      *slog.Logger
	identify func(T) string // non-nil enables dedup by the returned key

	errMu sync.Mutex
	err   error

	recent    []string
	recentSet map[string]struct{}
}

func newWSCore[T any](cfg Config, opts WebSocketOptions, url string, identify func(T) string) *wsCore[T] {
	cfg = cfg.withDefaults()
	return &wsCore[T]{
		cfg:       cfg,
		opts:      opts.withDefaults(),
		url:       url,
		log:       cfg.Logger,
		identify:  identify,
		recentSet: make(map[string]struct{}),
	}
}

func (w *wsCore[T]) stream(ctx context.Context, payload any) func(yield func(T) bool) {
	return func(yield func(T) bool) {
		w.setErr(w.run(ctx, payload, yield))
	}
}

func (w *wsCore[T]) setErr(err error) {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	w.err = err
}

func (w *wsCore[T]) getErr() error {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.err
}

// run is the reconnect loop. It returns nil on normal shutdown (context
// cancelled, consumer broke out of the loop, or server preemption) and a
// terminal error such as ErrBlocked otherwise.
func (w *wsCore[T]) run(ctx context.Context, payload any, yield func(T) bool) error {
	delay := w.opts.BaseReconnectDelay
	var reconnectAt time.Time

	for {
		if ctx.Err() != nil {
			return nil
		}
		w.log.Info("finlight ws: connecting", "url", w.url)
		res := w.runConnection(ctx, payload, yield, &reconnectAt)
		if res.terminal {
			return res.err
		}
		if ctx.Err() != nil {
			return nil
		}
		if res.connected {
			delay = w.opts.BaseReconnectDelay
		}

		wait := delay
		if now := time.Now(); reconnectAt.After(now) {
			wait = reconnectAt.Sub(now)
			w.log.Info("finlight ws: waiting until reconnectAt", "wait", wait)
		} else {
			w.log.Info("finlight ws: reconnecting", "delay", wait)
			delay = min(delay*2, w.opts.MaxReconnectDelay)
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil
		}
	}
}

type connResult struct {
	connected bool // the connection was established
	terminal  bool // stop the reconnect loop
	err       error
}

func (w *wsCore[T]) runConnection(ctx context.Context, payload any, yield func(T) bool, reconnectAt *time.Time) connResult {
	header := http.Header{}
	header.Set("x-api-key", w.cfg.APIKey)
	header.Set("x-client-version", clientVersion())
	if w.opts.Takeover {
		header.Set("x-takeover", "true")
	}

	dialCtx, cancelDial := context.WithTimeout(ctx, w.cfg.Timeout)
	conn, resp, err := websocket.Dial(dialCtx, w.url, &websocket.DialOptions{HTTPHeader: header})
	cancelDial()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			*reconnectAt = time.Now().Add(dialRateLimitBackoff)
			w.log.Warn("finlight ws: server rejected connection (429)", "backoff", dialRateLimitBackoff)
		} else {
			w.log.Error("finlight ws: connection failed", "err", err)
		}
		return connResult{}
	}
	conn.SetReadLimit(maxArticleMessageSize)
	defer conn.Close(websocket.StatusNormalClosure, "")

	w.log.Info("finlight ws: connected")
	*reconnectAt = time.Time{}

	c := &wsConn[T]{core: w, conn: conn, reconnectAt: reconnectAt, nonce: newUUIDv4()}
	c.lastPong.Store(time.Now().UnixMilli())

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.ctx = connCtx

	first, err := withClientNonce(payload, c.nonce)
	if err != nil {
		return connResult{connected: true, terminal: true, err: err}
	}
	if err := c.write(first); err != nil {
		w.log.Error("finlight ws: handshake write failed", "err", err)
		return connResult{connected: true}
	}

	go c.keepalive()

	return c.readLoop(yield)
}

// wsConn is the state of one established connection.
type wsConn[T any] struct {
	core        *wsCore[T]
	conn        *websocket.Conn
	ctx         context.Context
	writeMu     sync.Mutex
	lastPong    atomic.Int64 // unix millis
	nonce       string
	reconnectAt *time.Time
}

func (c *wsConn[T]) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, data)
}

// keepalive sends application-level pings, watches for missing pongs, and
// rotates the connection before the server-side lifetime cap.
func (c *wsConn[T]) keepalive() {
	ping := time.NewTicker(c.core.opts.PingInterval)
	defer ping.Stop()
	watchdog := time.NewTicker(watchdogInterval)
	defer watchdog.Stop()
	rotation := time.NewTimer(c.core.opts.ConnectionLifetime)
	defer rotation.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ping.C:
			if err := c.write(map[string]any{"action": "ping", "t": time.Now().UnixMilli()}); err != nil {
				c.core.log.Debug("finlight ws: ping failed", "err", err)
			}
		case <-watchdog.C:
			if time.Now().UnixMilli()-c.lastPong.Load() > c.core.opts.PongTimeout.Milliseconds() {
				c.core.log.Warn("finlight ws: no pong received in time, forcing reconnect")
				_ = c.conn.Close(websocket.StatusNormalClosure, "pong timeout")
				return
			}
		case <-rotation.C:
			c.core.log.Info("finlight ws: proactive rotation before server connection cap")
			_ = c.conn.Close(closeProactiveRotation, "Proactive rotation")
			return
		}
	}
}

func (c *wsConn[T]) readLoop(yield func(T) bool) connResult {
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			code := websocket.CloseStatus(err)
			reason := ""
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				reason = ce.Reason
			}
			c.core.log.Info("finlight ws: connection closed", "code", int(code), "reason", reason)
			c.notifyClose(int(code), reason)
			if code == websocket.StatusPolicyViolation { // 1008: blocked by server
				c.core.log.Warn("finlight ws: connection rejected by server (blocked)")
				return connResult{connected: true, terminal: true, err: ErrBlocked}
			}
			return connResult{connected: true}
		}
		if stop := c.handleMessage(data, yield); stop {
			c.notifyClose(int(websocket.StatusNormalClosure), "client stopped")
			return connResult{connected: true, terminal: true}
		}
	}
}

// handleMessage dispatches one server message. It returns true when the
// stream must end (server preemption or the consumer broke out of the loop).
func (c *wsConn[T]) handleMessage(data []byte, yield func(T) bool) (stop bool) {
	log := c.core.log
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Error("finlight ws: cannot parse message", "err", err)
		return false
	}

	switch msg.Action {
	case "pong":
		if msg.T > 0 {
			log.Debug("finlight ws: pong received", "rtt", time.Duration(time.Now().UnixMilli()-msg.T)*time.Millisecond)
		} else {
			log.Debug("finlight ws: pong received")
		}
		c.lastPong.Store(time.Now().UnixMilli())

	case "admit":
		log.Info("finlight ws: admitted", "leaseId", msg.LeaseID)
		if msg.ClientNonce != "" && msg.ClientNonce != c.nonce {
			log.Warn("finlight ws: nonce mismatch", "expected", c.nonce, "got", msg.ClientNonce)
		}

	case "preempted":
		log.Warn("finlight ws: connection preempted", "reason", msg.Reason, "newLeaseId", msg.NewLeaseID)
		_ = c.conn.Close(websocket.StatusNormalClosure, "Preempted by server")
		return true

	case "sendArticle":
		var article T
		if err := json.Unmarshal(msg.Data, &article); err != nil {
			log.Error("finlight ws: cannot parse article", "err", err)
			return false
		}
		if c.core.identify != nil {
			id := c.core.identify(article)
			if c.core.isDuplicate(id) {
				log.Debug("finlight ws: skipping duplicate article", "id", id)
				return false
			}
			c.core.track(id)
		}
		if !yield(article) {
			_ = c.conn.Close(websocket.StatusNormalClosure, "client stopped")
			return true
		}

	case "admin_kick":
		retryAfter := defaultAdminKickRetry
		if msg.RetryAfter > 0 {
			retryAfter = time.Duration(msg.RetryAfter) * time.Millisecond
		}
		*c.reconnectAt = time.Now().Add(retryAfter)
		log.Warn("finlight ws: admin kick", "retryAfter", retryAfter)
		_ = c.conn.Close(closeAdminKick, "Admin kick")

	case "error":
		errText := rawToString(msg.Data)
		if errText == "" {
			errText = rawToString(msg.Error)
		}
		log.Error("finlight ws: server error", "error", errText)
		switch {
		case strings.Contains(strings.ToLower(errText), "limit"):
			*c.reconnectAt = time.Now().Add(errorRateLimitBackoff)
			_ = c.conn.Close(closeRateLimited, "Rate limited")
		case strings.Contains(strings.ToLower(errText), "blocked"):
			*c.reconnectAt = time.Now().Add(errorBlockedBackoff)
			_ = c.conn.Close(closeUserBlocked, "User blocked")
		}

	default:
		log.Warn("finlight ws: unknown message action", "action", msg.Action)
	}
	return false
}

func (c *wsConn[T]) notifyClose(code int, reason string) {
	onClose := c.core.opts.OnClose
	if onClose == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.core.log.Error("finlight ws: OnClose callback panicked", "panic", r)
		}
	}()
	onClose(code, reason)
}

// isDuplicate / track implement the FIFO cache of recently seen article keys.
func (w *wsCore[T]) isDuplicate(id string) bool {
	_, ok := w.recentSet[id]
	return ok
}

func (w *wsCore[T]) track(id string) {
	w.recent = append(w.recent, id)
	w.recentSet[id] = struct{}{}
	if len(w.recent) > recentArticleCacheSize {
		delete(w.recentSet, w.recent[0])
		w.recent = w.recent[1:]
	}
}

// withClientNonce merges the handshake nonce into the request payload.
func withClientNonce(payload any, nonce string) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["clientNonce"] = nonce
	return m, nil
}

// rawToString renders a JSON value that may be a string or arbitrary JSON.
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
