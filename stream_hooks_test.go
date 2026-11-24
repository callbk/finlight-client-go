package finlight

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var admittedFrame = map[string]any{"event": "connection.admitted", "payloadVersion": 3, "data": map[string]any{"leaseId": "l", "serverNow": 1, "clientNonce": nil}}

// R-WS-6: Close called from inside any hook returns and ends the stream.
func TestCloseFromEachHook(t *testing.T) {
	cases := []struct {
		name  string
		opts  func(closeIt func()) *StreamOptions
		drive func(t *testing.T, ws *wsMock, d *dialInfo)
		pre   []dialReply
	}{
		{
			name: "OnAdmitted",
			opts: func(closeIt func()) *StreamOptions {
				return &StreamOptions{OnAdmitted: func(AdmittedInfo) { closeIt() }}
			},
			drive: func(t *testing.T, ws *wsMock, d *dialInfo) { d.conn.send(t, admittedFrame) },
		},
		{
			name: "OnError",
			opts: func(closeIt func()) *StreamOptions {
				return &StreamOptions{OnError: func(Error) { closeIt() }}
			},
			drive: func(t *testing.T, ws *wsMock, d *dialInfo) {
				d.conn.send(t, map[string]any{"event": "connection.error", "payloadVersion": 3, "data": map[string]any{"code": "INTERNAL_ERROR", "message": "x"}})
			},
		},
		{
			name: "OnReconnect",
			opts: func(closeIt func()) *StreamOptions {
				return &StreamOptions{OnReconnect: func(ReconnectInfo) { closeIt() }}
			},
			drive: func(t *testing.T, ws *wsMock, d *dialInfo) {
				d.conn.close()
				ws.nextDial(t, "redial").conn.nextNonPing(t, "re-subscribe")
			},
		},
		{
			name: "OnQuotaWait",
			opts: func(closeIt func()) *StreamOptions {
				return &StreamOptions{OnQuotaWait: func(QuotaWaitInfo) { closeIt() }}
			},
			pre: []dialReply{{status: 403, body: `{"message":"WebSocket quota exceeded"}`}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := newWSMock(t)
			ws.push(tc.pre...)
			c := newTestClient(t, Config{WSSURL: ws.URL(), Clock: systemClock{}, ReconnectBaseDelay: time.Millisecond, ReconnectMaxDelay: 2 * time.Millisecond})
			var s *Stream
			returned := make(chan struct{})
			var once sync.Once
			closeIt := func() {
				_ = s.Close()
				once.Do(func() { close(returned) })
			}
			var err error
			s, err = c.Stream(t.Context(), StreamParams{}, tc.opts(closeIt))
			if err != nil {
				t.Fatal(err)
			}
			ended := make(chan error, 1)
			go func() {
				var last error
				for _, err := range s.Events() {
					last = err
				}
				ended <- last
			}()
			d := ws.nextDial(t, "dial")
			if d.conn != nil {
				d.conn.nextNonPing(t, "subscribe")
			}
			if tc.drive != nil {
				tc.drive(t, ws, d)
			}
			select {
			case <-returned:
			case <-time.After(3 * time.Second):
				t.Fatal("Close called from the hook did not return")
			}
			select {
			case err := <-ended:
				if err != nil {
					t.Errorf("Close from a hook ended the stream with %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("the stream did not end")
			}
			if !s.Closed() {
				t.Error("Closed() = false")
			}
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}

// R-WS-4: a hook that blocks longer than the watchdog does not stop pings and
// pongs, so it never forces a reconnect.
func TestSlowHookDoesNotTriggerWatchdog(t *testing.T) {
	ws := newWSMock(t)
	release := make(chan struct{})
	c := newTestClient(t, Config{WSSURL: ws.URL(), Clock: systemClock{}, PingInterval: 20 * time.Millisecond, WatchdogTimeout: 100 * time.Millisecond})
	s, _ := c.Stream(t.Context(), StreamParams{}, &StreamOptions{OnAdmitted: func(AdmittedInfo) { <-release }})
	var pongs atomic.Int32
	s.hooks.onPong = func() { pongs.Add(1) }
	go func() {
		for range s.Events() {
		}
	}()
	d := ws.nextDial(t, "dial")
	d.conn.nextNonPing(t, "subscribe")
	d.conn.send(t, admittedFrame)
	time.Sleep(500 * time.Millisecond)
	if n := ws.count.Load(); n != 1 {
		t.Errorf("dials = %d while a hook was blocked, want 1", n)
	}
	if n := pongs.Load(); n < 5 {
		t.Errorf("pongs = %d while a hook was blocked", n)
	}
	close(release)
	_ = s.Close()
}

// R-WS-2: connection.admitted comes before the search, so an admit followed by
// SEARCH_FAILED must not reset the backoff. A healthy connection does.
func TestBackoffGrowsOverSearchFailed(t *testing.T) {
	ws := newWSMock(t)
	clock := newFakeClock()
	c := newTestClient(t, Config{WSSURL: ws.URL(), Clock: clock, SearchFailedThreshold: 100})
	s, _ := c.Stream(t.Context(), StreamParams{}, nil)
	delays := make(chan time.Duration, 16)
	s.hooks.onScheduled = func(d time.Duration, r ReconnectReason) { delays <- d }
	go func() {
		for range s.Events() {
		}
	}()
	defer s.Close()
	next := func() time.Duration {
		t.Helper()
		select {
		case d := <-delays:
			return d
		case <-time.After(3 * time.Second):
			t.Fatal("no reconnect scheduled")
		}
		return 0
	}
	conn := ws.nextDial(t, "dial").conn
	for i := range 6 {
		conn.nextNonPing(t, "subscribe")
		conn.send(t, admittedFrame)
		conn.send(t, map[string]any{"event": "connection.error", "payloadVersion": 3, "data": map[string]any{"code": "SEARCH_FAILED", "message": "x"}})
		d := next()
		low := min(time.Second<<i, 30*time.Second) / 2
		if d < low {
			t.Errorf("failure %d: delay = %v, want at least %v", i+1, d, low)
		}
		clock.Advance(d, nil)
		conn = ws.nextDial(t, "redial").conn
	}
	// A delivered article proves the connection healthy: the next drop
	// starts the backoff over.
	conn.nextNonPing(t, "subscribe")
	conn.send(t, admittedFrame)
	conn.send(t, json.RawMessage(`{"event":"article.created","payloadVersion":3,"data":`+article+`}`))
	time.Sleep(20 * time.Millisecond)
	conn.close()
	if d := next(); d > time.Second {
		t.Errorf("delay after a healthy connection = %v, want the first backoff step", d)
	}
}
