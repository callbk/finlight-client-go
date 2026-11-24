package finlight

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// StreamEvent is one item of a stream. Kind is created or updated for article
// frames (Data is set) and other for frames of an unknown event (Data is nil;
// RawJSON has the frame).
type StreamEvent struct {
	Kind  StreamEventKind
	Event StreamEventName // the wire event name (open)
	Data  *Article

	raw json.RawMessage
}

// RawJSON returns the whole frame as received.
func (e StreamEvent) RawJSON() json.RawMessage { return e.raw }

// AdmittedInfo is passed to OnAdmitted for every connection.admitted frame.
// Admitted comes before the subscribe is validated: it does not mean the
// subscribe was accepted.
type AdmittedInfo struct {
	LeaseID     string
	ServerNow   int64 // epoch milliseconds
	ClientNonce *string
}

// ReconnectInfo is passed to OnReconnect after every new connection except
// the first. LastSeenAt is the availableAt (enriched; firstSeenAt when
// availableAt is null) or firstSeenAt (raw) of the last delivered article, nil
// when nothing came yet. The SDK does not backfill; use Articles.Search with it.
type ReconnectInfo struct {
	LastSeenAt *string
	Reason     ReconnectReason
	Connection int // number of connections of this stream so far, the new one included
}

// QuotaWaitInfo is passed to OnQuotaWait for every wait on a new WebSocket
// quota period. Error is the *PreemptedError or the 403 *APIError.
type QuotaWaitInfo struct {
	Error         Error
	NextAttemptIn time.Duration
	Attempt       int // from 1
}

// StreamOptions are the options of Client.Stream and Client.RawStream.
//
// Hooks run one at a time, in order, on a goroutine of their own, never on the
// goroutine that reads the socket: a slow hook does not delay pings, pongs or
// the watchdog. Code shared between hooks and the Events loop needs its own
// locking. A panic in a hook is recovered and logged. A hook may call
// Stream.Close.
type StreamOptions struct {
	// Takeover sends x-takeover: true, which replaces the stalest existing
	// connection of the key when the connection limit is reached. Default off.
	Takeover bool
	// OnAdmitted is called for every connection.admitted frame.
	OnAdmitted func(AdmittedInfo)
	// OnReconnect is called after every new connection except the first.
	OnReconnect func(ReconnectInfo)
	// OnError receives problems that do not end the stream.
	OnError func(Error)
	// OnQuotaWait is called when the WebSocket quota is used up and the stream
	// waits for the next attempt (see Config.QuotaReconnectInterval).
	OnQuotaWait func(QuotaWaitInfo)
}

// Stream is a live article stream. It reconnects on its own (watchdog,
// rotation before the 2 h limit, backoff) and drops duplicate deliveries. It
// can be iterated once with Events. Stop it by cancelling its context,
// breaking out of the loop or calling Close.
type Stream struct {
	c    *Client
	raw  bool
	url  string
	sub  map[string]any
	opts StreamOptions
	ctx  context.Context

	mu          sync.Mutex
	started     bool
	closed      bool
	userClosed  bool
	lastSeenAt  *string
	connections int
	cancel      context.CancelFunc
	done        chan struct{}
	q           *eventQueue
	hq          *hookQueue

	hooks streamTestHooks
}

// streamTestHooks let the conformance tests observe the stream. All nil in
// normal use.
type streamTestHooks struct {
	nonce       func() string
	onFrame     func()
	onPong      func()
	onPingSent  func()
	onDuplicate func(id string)
	onScheduled func(delay time.Duration, reason ReconnectReason)
}

// Stream opens an enriched stream. Parameters are checked now; the connection
// opens when Events is iterated.
func (c *Client) Stream(ctx context.Context, params StreamParams, opts *StreamOptions) (*Stream, error) {
	sub, err := params.subscribe()
	if err != nil {
		return nil, err
	}
	return c.newStream(ctx, false, sub, opts), nil
}

// RawStream opens a raw stream: the article skeleton only (no content,
// sentiment, companies or events; availableAt always null).
func (c *Client) RawStream(ctx context.Context, params RawStreamParams, opts *StreamOptions) (*Stream, error) {
	sub, err := params.subscribe()
	if err != nil {
		return nil, err
	}
	return c.newStream(ctx, true, sub, opts), nil
}

func (c *Client) newStream(ctx context.Context, raw bool, sub map[string]any, opts *StreamOptions) *Stream {
	s := &Stream{c: c, raw: raw, sub: sub, ctx: ctx, done: make(chan struct{}), q: newEventQueue()}
	s.hq = newHookQueue(s.warn)
	s.url = c.cfg.WSSURL
	if raw {
		s.url += "/raw"
	}
	if opts != nil {
		s.opts = *opts
	}
	return s
}

// String never shows the API key.
func (s *Stream) String() string { return fmt.Sprintf("finlight.Stream{URL:%q}", s.url) }

// Events yields the stream's items. A terminal error (refused dial, VALIDATION_FAILED,
// preemption, a cancelled context) ends the iteration as the last pair
// (StreamEvent{}, err). Close ends it without an error. Iterating a second
// time yields a *ValidationError.
func (s *Stream) Events() iter.Seq2[StreamEvent, error] {
	return func(yield func(StreamEvent, error) bool) {
		s.mu.Lock()
		if s.started {
			s.mu.Unlock()
			yield(StreamEvent{}, validationErr("", "a stream can be iterated only once; open a new stream"))
			return
		}
		s.started = true
		if s.closed {
			s.mu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(s.ctx)
		s.cancel = cancel
		s.mu.Unlock()
		s.hq.start()
		go s.run(ctx)
		defer s.Close()
		for {
			item, ok := s.q.pop()
			if !ok {
				return
			}
			if item.err != nil {
				yield(StreamEvent{}, item.err)
				return
			}
			if !yield(item.ev, nil) {
				return
			}
		}
	}
}

// Close stops the stream: the socket is closed with 1000 and every timer and
// goroutine ends before Close returns. Hooks queued before Close still run
// and Close waits for them, except when a hook is running at that moment (for
// example when Close is called from inside a hook): then Close does not wait
// for the hooks, which end on their own right after. No hook is queued after
// Close. Safe to call more than once.
func (s *Stream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if s.started {
			<-s.waitDone()
			s.hq.wait()
		}
		return nil
	}
	s.closed = true
	s.userClosed = true
	cancel := s.cancel
	started := s.started
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started && cancel != nil {
		<-s.done
		s.hq.wait()
	}
	s.q.finish(nil)
	return nil
}

func (s *Stream) waitDone() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return s.done
}

// Closed reports whether the stream has ended (closed or failed).
func (s *Stream) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// LastSeenAt is the timestamp of the last delivered article (see ReconnectInfo).
func (s *Stream) LastSeenAt() *string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeenAt
}

// ---- runner ----

type outcomeAction int

const (
	actStop outcomeAction = iota
	actTerminal
	actReconnect // after delay
	actQuota
)

type outcome struct {
	action   outcomeAction
	reason   ReconnectReason
	delay    time.Duration
	backoff  bool // delay comes from the backoff schedule
	err      Error
	report   Error           // a refused or failed dial: the error for OnError
	next     *websocket.Conn // rotation: the next connection, opened and subscribed
	admitted bool
	// healthy: the connection delivered an article or a pong. Only then does
	// the reconnect backoff start over; connection.admitted comes before the
	// subscribe is validated and searched, so it proves nothing.
	healthy bool
}

func (s *Stream) warn(msg string, args ...any)  { s.c.cfg.Logger.Warn("finlight: "+msg, args...) }
func (s *Stream) debug(msg string, args ...any) { s.c.cfg.Logger.Debug("finlight: "+msg, args...) }

func (s *Stream) run(ctx context.Context) {
	defer close(s.done)
	defer s.hq.finish()
	cfg := s.c.cfg
	dd := newDedupe(cfg.DedupeCacheSize)
	var (
		reason       ReconnectReason
		delay        time.Duration
		backoffN     int
		quotaAttempt int
		pending      *websocket.Conn // rotation: opened and subscribed already
		searchFails  int
	)
	// next applies a non-terminal outcome; it returns false when the stream ends.
	next := func(o outcome) bool {
		switch o.action {
		case actStop:
			s.end(nil)
			return false
		case actTerminal:
			s.end(o.err)
			return false
		case actQuota:
			if cfg.DisableQuotaReconnect {
				s.end(o.err)
				return false
			}
			quotaAttempt++
			delay, reason = cfg.QuotaReconnectInterval, ReconnectReasonQuotaExceeded
			s.quotaWait(o.err, delay, quotaAttempt)
			return true
		}
		reason = o.reason
		switch {
		case o.next != nil:
			pending, delay = o.next, 0
		case o.backoff:
			backoffN++
			delay = backoffDelay(cfg.ReconnectBaseDelay, cfg.ReconnectMaxDelay, backoffN-1)
		default:
			delay = o.delay
		}
		return true
	}
	for {
		if reason != "" {
			d, r := delay, reason
			s.hook(func() {
				if s.hooks.onScheduled != nil {
					s.hooks.onScheduled(d, r)
				}
			})
		}
		if err := sleep(ctx, cfg.Clock, delay); err != nil {
			s.end(nil)
			return
		}
		conn, subscribed := pending, pending != nil
		pending = nil
		if conn == nil {
			var o outcome
			if conn, o = s.dial(ctx); conn == nil {
				if o.report != nil {
					s.reportError(o.report)
				}
				if !next(o) {
					return
				}
				continue
			}
		}
		s.mu.Lock()
		s.connections++
		n := s.connections
		last := s.lastSeenAt
		s.mu.Unlock()
		if n > 1 {
			info := ReconnectInfo{LastSeenAt: last, Reason: reason, Connection: n}
			if fn := s.opts.OnReconnect; fn != nil {
				s.hq.push(func() { fn(info) })
			}
		}
		o := s.session(ctx, conn, subscribed, dd, &searchFails)
		if o.healthy {
			backoffN = 0
		}
		if o.admitted {
			quotaAttempt = 0
		}
		if !next(o) {
			return
		}
	}
}

// end finishes the stream. A cancelled context ends with ctx.Err() unless the
// user closed the stream.
func (s *Stream) end(err Error) {
	s.mu.Lock()
	user := s.userClosed
	s.closed = true
	s.mu.Unlock()
	switch {
	case err != nil:
		s.warn("stream ended", "error", err.Error())
		s.q.finish(err)
	case user:
		s.q.finish(nil)
	case s.ctx.Err() != nil:
		s.q.finish(s.ctx.Err())
	default:
		s.q.finish(nil)
	}
}

func (s *Stream) quotaWait(err Error, next time.Duration, attempt int) {
	s.warn("WebSocket quota used up; waiting for the next attempt", "nextAttemptIn", next, "attempt", attempt)
	info := QuotaWaitInfo{Error: err, NextAttemptIn: next, Attempt: attempt}
	if fn := s.opts.OnQuotaWait; fn != nil {
		s.hq.push(func() { fn(info) })
	}
}

func (s *Stream) reportError(err Error) {
	if fn := s.opts.OnError; fn != nil {
		s.hq.push(func() { fn(err) })
	}
}

// hook runs test code inline and recovers a panic so that it never ends the
// stream. User hooks go through s.hq instead.
func (s *Stream) hook(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			s.warn("a stream hook panicked", "panic", fmt.Sprint(r))
		}
	}()
	fn()
}

func (s *Stream) dial(ctx context.Context) (*websocket.Conn, outcome) {
	cfg := s.c.cfg
	// The gateway looks the key up case-sensitively: assign the lowercase names
	// directly. Header.Set would canonicalize them to X-Api-Key.
	header := http.Header{}
	header["x-api-key"] = []string{s.c.key.reveal()}
	header["x-client-version"] = []string{Version}
	header["User-Agent"] = []string{userAgent}
	if s.opts.Takeover {
		header["x-takeover"] = []string{"true"}
	}
	dctx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	conn, resp, err := websocket.Dial(dctx, s.url, &websocket.DialOptions{HTTPHeader: header, HTTPClient: cfg.HTTPClient})
	if err == nil {
		conn.SetReadLimit(cfg.MaxMessageBytes)
		s.debug("stream connected", "url", s.url)
		return conn, outcome{}
	}
	if ctx.Err() != nil {
		return nil, outcome{action: actStop}
	}
	if resp == nil || resp.StatusCode == http.StatusSwitchingProtocols {
		nerr := &NetworkError{Message: "stream dial failed", Err: err}
		s.warn("stream dial failed", "error", err.Error())
		return nil, outcome{action: actReconnect, reason: ReconnectReasonDialFailed, backoff: true, report: nerr}
	}
	var body []byte
	if resp.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}
	return nil, s.classifyDial(resp.StatusCode, body)
}

// dialAPIError builds the error of a refused dial; the kind comes from the status.
func dialAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status, Raw: string(body), APIKind: apiKindForStatus(status)}
	if status != 401 && status != 403 && status != 429 && status < 500 {
		e.APIKind = APIKindOther
	}
	var o jsonObject
	if json.Unmarshal(body, &o) == nil {
		e.rawJSON = append(json.RawMessage(nil), body...)
		for _, k := range []string{"message", "error", "detail"} {
			if v, _ := o.plainString("", k); v != "" {
				e.Detail = v
				break
			}
		}
	} else {
		e.Detail = strings.TrimSpace(string(body))
	}
	if e.Detail == "" {
		e.Detail = http.StatusText(status)
	}
	return e
}

// classifyDial sorts a refused dial (R-WS-2). The 403 texts are compared by
// their beginning; without a readable body, 403 waits long and reports. The
// error to report is in outcome.report; the caller reports it.
func (s *Stream) classifyDial(status int, body []byte) outcome {
	e := dialAPIError(status, body)
	msg := e.Detail
	s.warn("stream dial refused", "status", status, "message", msg)
	switch {
	case status == 401:
		return outcome{action: actTerminal, err: e}
	case status == 403 && strings.HasPrefix(msg, "Your subscription does not include WebSockets"):
		return outcome{action: actTerminal, err: e}
	case status == 403 && strings.HasPrefix(msg, "WebSocket quota exceeded"):
		return outcome{action: actQuota, err: e}
	case status == 403 && strings.HasPrefix(msg, "Too many concurrent connections"):
		// The error carries the x-takeover hint when the server gives one.
		return outcome{action: actReconnect, reason: ReconnectReasonConnectionLimit, delay: s.c.cfg.LongBackoff, report: e}
	case status == 403:
		return outcome{action: actReconnect, reason: ReconnectReasonDialRefused, delay: s.c.cfg.LongBackoff, report: e}
	case status == 429:
		return outcome{action: actReconnect, reason: ReconnectReasonRateLimited, delay: s.c.cfg.LongBackoff, report: e}
	case status >= 500:
		return outcome{action: actReconnect, reason: ReconnectReasonDialFailed, backoff: true, report: e}
	}
	return outcome{action: actReconnect, reason: ReconnectReasonDialRefused, backoff: true, report: e}
}

type readResult struct {
	data []byte
	err  error
}

func (s *Stream) newNonce() string {
	if s.hooks.nonce != nil {
		return s.hooks.nonce()
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "c-" + hex.EncodeToString(b[:])
}

// subscribeFrame builds the subscribe from the fixed key list of the params
// plus payloadVersion (the JSON integer 3) and a clientNonce per connection.
func (s *Stream) subscribeFrame() []byte {
	m := maps.Clone(s.sub)
	m["payloadVersion"] = PayloadVersion
	m["clientNonce"] = s.newNonce()
	data, _ := json.Marshal(m)
	return data
}

const writeTimeout = 10 * time.Second

func writeText(conn *websocket.Conn, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, data)
}

// session runs one connection until it ends and says what to do next. The
// reader and writers use their own contexts, so that a cancelled stream still
// closes its socket with 1000 instead of dropping it.
func (s *Stream) session(ctx context.Context, conn *websocket.Conn, subscribed bool, dd *dedupe, searchFails *int) outcome {
	cfg := s.c.cfg
	clock := cfg.Clock
	closed := false
	stopping := make(chan struct{})
	closeConn := func(code websocket.StatusCode) {
		if closed {
			return
		}
		closed = true
		close(stopping)
		if code == websocket.StatusNormalClosure {
			_ = conn.Close(code, "")
		} else {
			_ = conn.CloseNow()
		}
	}
	defer closeConn(websocket.StatusNormalClosure)

	// coder/websocket sends no protocol-level pings by itself; this application
	// ping (R-WS-4) is the only keep-alive.
	ping := clock.NewTicker(cfg.PingInterval)
	defer ping.Stop()
	watchdog := clock.NewTimer(cfg.WatchdogTimeout)
	defer func() { watchdog.Stop() }()
	rotation := clock.NewTimer(cfg.RotationInterval)
	defer rotation.Stop()

	rctx, rcancel := context.WithCancel(context.Background())
	msgs := make(chan readResult)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			_, data, err := conn.Read(rctx)
			select {
			case msgs <- readResult{data, err}:
			case <-stopping:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		closeConn(websocket.StatusNormalClosure)
		rcancel()
		<-readerDone
	}()

	if !subscribed {
		if err := writeText(conn, s.subscribeFrame()); err != nil {
			closeConn(websocket.StatusGoingAway)
			if ctx.Err() != nil {
				return outcome{action: actStop}
			}
			return outcome{action: actReconnect, reason: ReconnectReasonClosed, backoff: true}
		}
	}

	admitted, healthy := false, false
	for {
		select {
		case <-ctx.Done():
			closeConn(websocket.StatusNormalClosure)
			return outcome{action: actStop, admitted: admitted, healthy: healthy}
		case <-ping.C():
			msg := fmt.Sprintf(`{"action":"ping","t":%d}`, clock.Now().UnixMilli())
			if err := writeText(conn, []byte(msg)); err == nil {
				s.hook(func() {
					if s.hooks.onPingSent != nil {
						s.hooks.onPingSent()
					}
				})
			}
		case <-watchdog.C():
			s.warn("no pong within the watchdog timeout; reconnecting", "timeout", cfg.WatchdogTimeout)
			closeConn(websocket.StatusGoingAway)
			return outcome{action: actReconnect, reason: ReconnectReasonWatchdog, admitted: admitted, healthy: healthy}
		case <-rotation.C():
			// Make before break: open and subscribe the next connection, then
			// close this one.
			s.debug("rotating the stream connection")
			nextConn, o := s.dial(ctx)
			if nextConn == nil {
				// Close waits for the close reply (at most 5 s).
				closeConn(websocket.StatusNormalClosure)
				o.admitted, o.healthy = admitted, healthy
				if o.action == actStop || o.action == actTerminal || (o.action == actQuota && cfg.DisableQuotaReconnect) {
					return o
				}
				// Typically the plan's connection limit is in use by this very
				// connection. Now that it is closed, dial again at once, without
				// OnError; only a refusal of that dial waits and reports.
				s.debug("the rotation dial failed; closed the old connection, dialing again")
				return outcome{action: actReconnect, reason: ReconnectReasonRotation, admitted: admitted, healthy: healthy}
			}
			if err := writeText(nextConn, s.subscribeFrame()); err != nil {
				_ = nextConn.CloseNow()
				closeConn(websocket.StatusNormalClosure)
				return outcome{action: actReconnect, reason: ReconnectReasonRotation, admitted: admitted, healthy: healthy}
			}
			closeConn(websocket.StatusNormalClosure)
			return outcome{action: actReconnect, reason: ReconnectReasonRotation, next: nextConn, admitted: admitted, healthy: healthy}
		case m := <-msgs:
			if m.err != nil {
				closeConn(websocket.StatusGoingAway)
				if ctx.Err() != nil {
					return outcome{action: actStop, admitted: admitted, healthy: healthy}
				}
				if websocket.CloseStatus(m.err) == websocket.StatusPolicyViolation {
					return outcome{action: actTerminal, admitted: admitted, err: &APIError{
						APIKind: APIKindOther,
						Detail:  "the server closed the connection with 1008 (policy violation)",
						cause:   m.err,
					}}
				}
				s.warn("stream connection closed; reconnecting", "error", m.err.Error())
				return outcome{action: actReconnect, reason: ReconnectReasonClosed, backoff: true, admitted: admitted, healthy: healthy}
			}
			res, done := s.handleFrame(m.data, dd, searchFails, &admitted, &healthy, &watchdog)
			s.hook(func() {
				if s.hooks.onFrame != nil {
					s.hooks.onFrame()
				}
			})
			if done {
				closeConn(websocket.StatusNormalClosure)
				res.admitted, res.healthy = admitted, healthy
				return res
			}
		}
	}
}

// handleFrame processes one frame. Control frames are handled here, before any
// user code runs; articles go to the queue. done=true ends the connection.
func (s *Stream) handleFrame(data []byte, dd *dedupe, searchFails *int, admitted, healthy *bool, watchdog *Timer) (outcome, bool) {
	cfg := s.c.cfg
	f := decodeFrame(data)
	switch f.kind {
	case frameArticle:
		*searchFails = 0
		*healthy = true
		key := dedupeKey(f.event, f.article.ID, f.dataRaw)
		if dd.seen(key) {
			id := f.article.ID
			s.debug("duplicate delivery dropped", "event", string(f.event), "id", id)
			s.hook(func() {
				if s.hooks.onDuplicate != nil {
					s.hooks.onDuplicate(id)
				}
			})
			return outcome{}, false
		}
		kind := StreamEventKindCreated
		if f.event == StreamEventNameArticleUpdated {
			kind = StreamEventKindUpdated
		}
		seen := f.article.FirstSeenAt
		if !s.raw && f.article.AvailableAt != nil {
			seen = f.article.AvailableAt
		}
		if seen != nil {
			v := *seen
			s.mu.Lock()
			s.lastSeenAt = &v
			s.mu.Unlock()
		}
		s.q.push(StreamEvent{Kind: kind, Event: f.event, Data: f.article, raw: f.raw})
	case frameAdmitted:
		*admitted = true
		info := f.admitted
		if fn := s.opts.OnAdmitted; fn != nil {
			s.hq.push(func() { fn(info) })
		}
	case framePong:
		*healthy = true
		(*watchdog).Stop()
		*watchdog = cfg.Clock.NewTimer(cfg.WatchdogTimeout)
		s.hook(func() {
			if s.hooks.onPong != nil {
				s.hooks.onPong()
			}
		})
	case framePreempted:
		p := f.preempted
		switch p.Reason {
		case PreemptReasonConnectionLimit:
			return outcome{action: actTerminal, err: p}, true
		case PreemptReasonQuotaExceeded:
			return outcome{action: actQuota, err: p}, true
		}
		s.warn("connection preempted for an unknown reason; reconnecting", "reason", string(p.Reason))
		s.reportError(p)
		return outcome{action: actReconnect, reason: ReconnectReasonClosed, backoff: true}, true
	case frameError:
		e := &APIError{
			Code:    ProblemCode(f.errCode),
			Detail:  f.errMsg,
			Errors:  f.errIssues,
			APIKind: APIKindOther,
			Raw:     string(f.raw),
			rawJSON: f.raw,
		}
		switch f.errCode {
		case ConnectionErrorCodeValidationFailed:
			e.APIKind = APIKindValidationFailed
			return outcome{action: actTerminal, err: e}, true
		case ConnectionErrorCodeHandshakeFailed:
			s.warn("stream handshake failed; reconnecting")
			return outcome{action: actReconnect, reason: ReconnectReasonHandshakeFailed, backoff: true}, true
		case ConnectionErrorCodeSearchFailed:
			*searchFails++
			s.warn("stream search failed; reconnecting", "consecutive", *searchFails)
			if *searchFails >= cfg.SearchFailedThreshold {
				s.reportError(e)
			}
			return outcome{action: actReconnect, reason: ReconnectReasonSearchFailed, backoff: true}, true
		}
		s.warn("stream reported an error", "code", string(f.errCode), "message", f.errMsg)
		s.reportError(e)
	case frameOther:
		s.debug("frame with an unknown event", "event", string(f.event))
		s.q.push(StreamEvent{Kind: StreamEventKindOther, Event: f.event, raw: f.raw})
	default:
		if f.decodeErr != nil {
			s.warn("stream frame could not be decoded", "reason", f.note)
			var de *DecodeError
			if errors.As(f.decodeErr, &de) {
				s.reportError(de)
			}
		} else {
			s.debug("stream frame ignored", "reason", f.note)
		}
	}
	return outcome{}, false
}

// ---- dedupe ----

// dedupe is a bounded set of recent (event, data.id, sha256(data)) keys. It
// belongs to one stream and survives its reconnects and rotations.
type dedupe struct {
	size  int
	order []string
	set   map[string]struct{}
}

func newDedupe(size int) *dedupe { return &dedupe{size: size, set: map[string]struct{}{}} }

func dedupeKey(event StreamEventName, id string, data json.RawMessage) string {
	h := sha256.Sum256(data)
	return string(event) + "\x00" + id + "\x00" + hex.EncodeToString(h[:])
}

// seen reports whether key was seen before and records it.
func (d *dedupe) seen(key string) bool {
	if _, ok := d.set[key]; ok {
		return true
	}
	d.set[key] = struct{}{}
	d.order = append(d.order, key)
	if len(d.order) > d.size {
		delete(d.set, d.order[0])
		d.order = d.order[1:]
	}
	return false
}

// ---- queue ----

type queueItem struct {
	ev  StreamEvent
	err error
}

// eventQueue decouples the connection goroutine from the consumer, so that a
// slow consumer never delays pongs and the watchdog.
type eventQueue struct {
	mu     sync.Mutex
	items  []queueItem
	done   bool
	notify chan struct{}
}

func newEventQueue() *eventQueue { return &eventQueue{notify: make(chan struct{}, 1)} }

func (q *eventQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *eventQueue) push(ev StreamEvent) {
	q.mu.Lock()
	if !q.done {
		q.items = append(q.items, queueItem{ev: ev})
	}
	q.mu.Unlock()
	q.signal()
}

// finish ends the queue, with a terminal error when err is not nil. Items
// already queued are still delivered first.
func (q *eventQueue) finish(err error) {
	q.mu.Lock()
	if !q.done {
		q.done = true
		if err != nil {
			q.items = append(q.items, queueItem{err: err})
		}
	}
	q.mu.Unlock()
	q.signal()
}

func (q *eventQueue) pop() (queueItem, bool) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			it := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return it, true
		}
		done := q.done
		q.mu.Unlock()
		if done {
			return queueItem{}, false
		}
		<-q.notify
	}
}

// ---- hooks ----

// maxPendingHooks bounds the hooks waiting behind a slow one. Further hooks
// are dropped with a warning instead of growing without limit.
const maxPendingHooks = 256

// hookQueue runs the user hooks of one stream one at a time, in order, on a
// goroutine of their own. The connection goroutine only queues them, so a
// slow hook never delays pongs or the watchdog, and a hook can call Close
// without waiting for itself.
type hookQueue struct {
	warn func(msg string, args ...any)

	mu       sync.Mutex
	fns      []func()
	finished bool // no more hooks come; run the queued ones, then stop
	started  bool
	notify   chan struct{}
	done     chan struct{}
	running  atomic.Bool // a hook is running right now
}

func newHookQueue(warn func(msg string, args ...any)) *hookQueue {
	return &hookQueue{warn: warn, notify: make(chan struct{}, 1), done: make(chan struct{})}
}

func (h *hookQueue) signal() {
	select {
	case h.notify <- struct{}{}:
	default:
	}
}

func (h *hookQueue) push(fn func()) {
	h.mu.Lock()
	switch {
	case h.finished:
	case len(h.fns) >= maxPendingHooks:
		h.mu.Unlock()
		h.warn("a stream hook is too slow; dropping a hook call", "pending", maxPendingHooks)
		return
	default:
		h.fns = append(h.fns, fn)
	}
	h.mu.Unlock()
	h.signal()
}

// finish says that no more hooks come. Queued hooks still run.
func (h *hookQueue) finish() {
	h.mu.Lock()
	h.finished = true
	h.mu.Unlock()
	h.signal()
}

// wait blocks until the hook goroutine has ended, unless a hook is running
// right now: that hook may be the caller (Close from inside a hook), and
// waiting for it would never end.
func (h *hookQueue) wait() {
	h.mu.Lock()
	started := h.started
	h.mu.Unlock()
	if !started || h.running.Load() {
		return
	}
	<-h.done
}

func (h *hookQueue) start() {
	h.mu.Lock()
	h.started = true
	h.mu.Unlock()
	go h.run()
}

func (h *hookQueue) run() {
	defer close(h.done)
	for {
		h.mu.Lock()
		if h.finished && len(h.fns) == 0 {
			h.mu.Unlock()
			return
		}
		if len(h.fns) == 0 {
			h.mu.Unlock()
			<-h.notify
			continue
		}
		fn := h.fns[0]
		h.fns = h.fns[1:]
		h.running.Store(true)
		h.mu.Unlock()
		h.call(fn)
		h.running.Store(false)
	}
}

func (h *hookQueue) call(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			h.warn("a stream hook panicked", "panic", fmt.Sprint(r))
		}
	}()
	fn()
}
