package finlight

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"
)

// ---- HTTP mock: a real server on 127.0.0.1 ----

type recordedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

type mockResponse struct {
	Status int
	Header map[string]string
	Body   any // JSON value, a string written as is, or nil for no body
}

type httpMock struct {
	srv *httptest.Server

	mu        sync.Mutex
	requests  []recordedRequest
	responses []mockResponse
	always    *mockResponse
}

func newHTTPMock(t testing.TB) *httpMock {
	m := &httpMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *httpMock) URL() string { return m.srv.URL }

func (m *httpMock) enqueue(r mockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, r)
}

func (m *httpMock) setAlways(r mockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.always = &r
}

// serveFixture enqueues a fixture "response" object.
func (m *httpMock) serveFixture(resp any) {
	m.enqueue(fixtureResponse(resp))
}

func fixtureResponse(resp any) mockResponse {
	r := obj(resp)
	out := mockResponse{Status: int(r["status"].(float64)), Body: r["body"], Header: map[string]string{}}
	for k, v := range obj(r["headers"]) {
		out.Header[k] = str(v)
	}
	return out
}

func (m *httpMock) reqs() []recordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recordedRequest(nil), m.requests...)
}

func (m *httpMock) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, recordedRequest{
		Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: body,
	})
	var resp mockResponse
	switch {
	case len(m.responses) > 0:
		resp = m.responses[0]
		m.responses = m.responses[1:]
	case m.always != nil:
		resp = *m.always
	default:
		resp = mockResponse{Status: 599, Body: "no response queued"}
	}
	m.mu.Unlock()
	for k, v := range resp.Header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	switch b := resp.Body.(type) {
	case nil:
	case string:
		_, _ = io.WriteString(w, b)
	case json.RawMessage:
		_, _ = w.Write(b)
	default:
		data, _ := json.Marshal(b)
		_, _ = w.Write(data)
	}
}

// bodyText is the exact text the mock wrote for a response body.
func bodyText(b any) string {
	switch v := b.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

// queryPairs decodes a query string into sorted name=value pairs.
func queryPairs(t testing.TB, raw string) []string {
	t.Helper()
	vals, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	var out []string
	for k, vs := range vals {
		for _, v := range vs {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}

// ---- clocks ----

// instantClock fires every timer at once and records the requested waits.
type instantClock struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (c *instantClock) Now() time.Time { return time.Date(2026, 11, 23, 21, 4, 5, 0, time.UTC) }

func (c *instantClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.mu.Unlock()
	ch := make(chan time.Time, 1)
	ch <- c.Now()
	return chanTimer{ch}
}

func (c *instantClock) NewTicker(time.Duration) Timer { return chanTimer{make(chan time.Time)} }

func (c *instantClock) sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

// stuckClock never fires.
type stuckClock struct{ started chan time.Duration }

func (c *stuckClock) Now() time.Time { return time.Unix(0, 0) }
func (c *stuckClock) NewTimer(d time.Duration) Timer {
	if c.started != nil {
		c.started <- d
	}
	return chanTimer{make(chan time.Time)}
}
func (c *stuckClock) NewTicker(time.Duration) Timer { return chanTimer{make(chan time.Time)} }

type chanTimer struct{ ch chan time.Time }

func (t chanTimer) C() <-chan time.Time { return t.ch }
func (t chanTimer) Stop()               {}

// fakeClock is a manual clock for streams: Advance fires due timers in order.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c       *fakeClock
	when    time.Time
	period  time.Duration
	ch      chan time.Time
	stopped bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 23, 21, 4, 5, 123e6, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) add(d, period time.Duration) *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, when: c.now.Add(d), period: period, ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) NewTimer(d time.Duration) Timer  { return c.add(d, 0) }
func (c *fakeClock) NewTicker(d time.Duration) Timer { return c.add(d, d) }

func (t *fakeTimer) C() <-chan time.Time { return t.ch }
func (t *fakeTimer) Stop() {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	t.stopped = true
	for i, x := range t.c.timers {
		if x == t {
			t.c.timers = append(t.c.timers[:i], t.c.timers[i+1:]...)
			break
		}
	}
}

// pending counts live timers.
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// waitTimer waits until a one-shot timer due in d is registered. The stream
// reports a scheduled reconnect before it starts the timer, so a test that
// advances the clock right after the report can otherwise miss the timer.
func (c *fakeClock) waitTimer(t testing.TB, d time.Duration) {
	t.Helper()
	waitFor(t, "timer for "+d.String(), func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, x := range c.timers {
			if x.period == 0 && !x.stopped && x.when.Equal(c.now.Add(d)) {
				return true
			}
		}
		return false
	})
}

// Advance moves the clock forward by d, firing due timers one at a time and
// calling settle after each so the stream can react (and register new timers).
func (c *fakeClock) Advance(d time.Duration, settle func(fired *fakeTimer)) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *fakeTimer
		for _, t := range c.timers {
			if !t.stopped && !t.when.After(target) && (next == nil || t.when.Before(next.when)) {
				next = t
			}
		}
		if next == nil {
			c.now = target
			c.mu.Unlock()
			return
		}
		c.now = next.when
		if next.period > 0 {
			next.when = next.when.Add(next.period)
		} else {
			next.stopped = true
			for i, x := range c.timers {
				if x == next {
					c.timers = append(c.timers[:i], c.timers[i+1:]...)
					break
				}
			}
		}
		select {
		case next.ch <- c.now:
		default:
		}
		c.mu.Unlock()
		if settle != nil {
			settle(next)
		}
	}
}

// waitFor polls cond for up to 5 s.
func waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func newTestClient(t testing.TB, cfg Config) *Client {
	t.Helper()
	if cfg.APIKey == "" {
		cfg.APIKey = kitAPIKey
	}
	if cfg.Clock == nil {
		cfg.Clock = &instantClock{}
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
