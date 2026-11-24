package finlight

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// probe runs a stream against the wsMock and records everything a session
// fixture can expect.
type probe struct {
	t     testing.TB
	ws    *wsMock
	clock *fakeClock

	mu         sync.Mutex
	items      []StreamEvent
	admitted   []AdmittedInfo
	reconnects []ReconnectInfo
	errs       []Error
	scheduled  []scheduled
	duplicates []string
	quotaWaits []QuotaWaitInfo
	frames     int
	pongs      int
	pings      int
	terminal   error
	done       bool

	client   *Client
	stream   *Stream
	finished chan struct{}
}

type scheduled struct {
	delay  time.Duration
	reason ReconnectReason
	at     time.Time
}

func (p *probe) with(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn()
}

type probeOptions struct {
	raw    bool
	params map[string]any
	apiKey string
	nonce  string
	config Config
}

func startProbe(t testing.TB, ws *wsMock, o probeOptions) *probe {
	t.Helper()
	p := &probe{t: t, ws: ws, clock: newFakeClock(), finished: make(chan struct{})}
	cfg := o.config
	cfg.APIKey = o.apiKey
	cfg.WSSURL = ws.URL()
	cfg.BaseURL = "http://127.0.0.1:9"
	cfg.Clock = p.clock
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p.client = c
	opts := &StreamOptions{
		OnAdmitted:  func(a AdmittedInfo) { p.with(func() { p.admitted = append(p.admitted, a) }) },
		OnReconnect: func(r ReconnectInfo) { p.with(func() { p.reconnects = append(p.reconnects, r) }) },
		OnError:     func(e Error) { p.with(func() { p.errs = append(p.errs, e) }) },
		OnQuotaWait: func(q QuotaWaitInfo) { p.with(func() { p.quotaWaits = append(p.quotaWaits, q) }) },
	}
	if o.raw {
		p.stream, err = c.RawStream(t.Context(), rawParamsFrom(t, o.params), opts)
	} else {
		p.stream, err = c.Stream(t.Context(), streamParamsFrom(t, o.params), opts)
	}
	if err != nil {
		t.Fatal(err)
	}
	nonce := o.nonce
	p.stream.hooks = streamTestHooks{
		nonce:       func() string { return nonce },
		onFrame:     func() { p.with(func() { p.frames++ }) },
		onPong:      func() { p.with(func() { p.pongs++ }) },
		onPingSent:  func() { p.with(func() { p.pings++ }) },
		onDuplicate: func(id string) { p.with(func() { p.duplicates = append(p.duplicates, id) }) },
		onScheduled: func(d time.Duration, r ReconnectReason) {
			now := p.clock.Now()
			p.with(func() { p.scheduled = append(p.scheduled, scheduled{d, r, now}) })
		},
	}
	go func() {
		defer close(p.finished)
		for ev, err := range p.stream.Events() {
			if err != nil {
				p.with(func() { p.terminal = err })
				continue
			}
			p.with(func() { p.items = append(p.items, ev) })
		}
		p.with(func() { p.done = true })
	}()
	return p
}

func (p *probe) isDone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

func (p *probe) count(fn func() int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fn()
}

// advance moves the fake clock; after every ping it waits for the ping (and,
// with auto-pong, for its pong) so that the watchdog sees each pong in time.
func (p *probe) advance(d time.Duration) {
	p.clock.Advance(d, func(f *fakeTimer) {
		deadline := time.Now().Add(2 * time.Second)
		for len(f.ch) > 0 && time.Now().Before(deadline) {
			time.Sleep(200 * time.Microsecond)
		}
		if f.period > 0 && len(f.ch) == 0 {
			before := p.count(func() int { return p.pings })
			_ = before
			for time.Now().Before(deadline) {
				pings := p.count(func() int { return p.pings })
				pongs := p.count(func() int { return p.pongs })
				if pings > 0 && (!p.ws.autoPong.Load() || pongs >= pings) {
					break
				}
				time.Sleep(200 * time.Microsecond)
			}
		}
		time.Sleep(500 * time.Microsecond)
	})
}

func (p *probe) stop() {
	_ = p.stream.Close()
	<-p.finished
}

// paramsFrom fills a params struct from a subscribe frame by wire key; a key
// without a field fails the test.
func paramsFrom(t testing.TB, dst any, m map[string]any) {
	t.Helper()
	rv := reflect.ValueOf(dst).Elem()
	for k, v := range m {
		if k == "payloadVersion" || k == "clientNonce" {
			continue
		}
		f := rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, k) })
		if !f.IsValid() {
			t.Fatalf("%s has no field for %q", rv.Type().Name(), k)
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(str(v))
		case reflect.Slice:
			list := v.([]any)
			s := reflect.MakeSlice(f.Type(), len(list), len(list))
			for i, x := range list {
				s.Index(i).SetString(str(x))
			}
			f.Set(s)
		case reflect.Pointer:
			b := v.(bool)
			f.Set(reflect.ValueOf(&b))
		default:
			t.Fatalf("unhandled field %s", k)
		}
	}
}

func streamParamsFrom(t testing.TB, m map[string]any) StreamParams {
	var p StreamParams
	paramsFrom(t, &p, m)
	return p
}

func rawParamsFrom(t testing.TB, m map[string]any) RawStreamParams {
	var p RawStreamParams
	paramsFrom(t, &p, m)
	return p
}

func isSubscribe(f any) bool {
	m, ok := f.(map[string]any)
	if !ok {
		return false
	}
	_, pv := m["payloadVersion"]
	_, typ := m["type"]
	_, act := m["action"]
	return pv && !typ && !act
}

func without(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

type sessionOptions struct {
	forbidden []string // subscribe keys the SDK refuses to send
	autoPong  *bool
	after     func(t *testing.T, s *sessionState)
}

type sessionState struct {
	p         *probe
	ws        *wsMock
	subscribe map[string]any
	conn      *serverConn
	checked   counts
}

type counts struct{ admitted, items, pongs, scheduled, errs, duplicates, quota int }

// runSession plays a ws-sessions fixture against the SDK and checks every expect step.
func runSession(t *testing.T, rel string, o sessionOptions) {
	f := loadFixture(t, rel)
	steps := f["steps"].([]any)
	raw := f["endpoint"] == "raw"
	ws := newWSMock(t)
	if o.autoPong != nil {
		ws.autoPong.Store(*o.autoPong)
	}

	var first map[string]any
	for _, s := range steps {
		st := obj(s)
		if st["step"] == "clientSends" && isSubscribe(st["frame"]) {
			first = obj(st["frame"])
			break
		}
	}
	if first == nil {
		first = map[string]any{"payloadVersion": float64(3), "query": "ticker:XMPL"}
	}
	subscribe := without(first, o.forbidden...)
	params := without(subscribe, "payloadVersion", "clientNonce")
	var firstDial map[string]any
	for _, s := range steps {
		st := obj(s)
		if st["step"] == "dial" {
			if firstDial == nil {
				firstDial = st
			}
			srv := obj(st["server"])
			ws.push(dialReply{status: int(srv["status"].(float64)), body: str(srv["body"])})
		}
	}
	apiKey := kitAPIKey
	if k, ok := obj(firstDial["expectRequestHeaders"])["x-api-key"]; ok {
		apiKey = str(k)
	}
	nonce := str(first["clientNonce"])
	if nonce == "" {
		nonce = "c-test"
	}
	var cfg Config
	if cc := obj(f["clientConfig"]); cc != nil {
		if q, ok := cc["quotaReconnect"].(bool); ok {
			cfg.DisableQuotaReconnect = !q
		}
	}

	baseline := goroutines()
	p := startProbe(t, ws, probeOptions{raw: raw, params: params, apiKey: apiKey, nonce: nonce, config: cfg})
	st := &sessionState{p: p, ws: ws, subscribe: subscribe}
	var subscribedOn *serverConn

	for i, s := range steps {
		step := obj(s)
		switch step["step"] {
		case "dial":
			d := ws.nextDial(t, "dial")
			wantPath := "/"
			if raw {
				wantPath = "/raw"
			}
			if d.path != wantPath && (wantPath != "/" || d.path != "") {
				t.Errorf("dial path = %q, want %q", d.path, wantPath)
			}
			if strings.Contains(strings.ToLower(d.rawQuery), "apikey") {
				t.Errorf("API key in the URL: %s", d.rawQuery)
			}
			if k, ok := obj(step["expectRequestHeaders"])["x-api-key"]; ok && d.header.Get("x-api-key") != k {
				t.Errorf("x-api-key = %q", d.header.Get("x-api-key"))
			}
			if d.header.Get("x-client-version") != Version || d.header.Get("User-Agent") != "finlight-client-go/"+Version {
				t.Errorf("handshake headers: %v", d.header)
			}
			if d.header.Get("x-takeover") != "" {
				t.Error("takeover must be off by default")
			}
			st.conn = d.conn
		case "clientSends":
			if match := obj(step["match"]); match != nil {
				got := st.conn.next(t, "client frame matching "+string(mustJSON(t, match)))
				for k, v := range match {
					if got[k] != v {
						t.Errorf("frame %v does not match %v", got, match)
					}
				}
				if match["action"] == "ping" {
					if _, ok := got["t"].(float64); !ok {
						t.Errorf("ping t is not a number: %v", got)
					}
				}
				continue
			}
			frame := step["frame"]
			if _, isString := frame.(string); isString {
				continue // a broken frame no SDK sends
			}
			if obj(frame)["action"] == "ping" {
				st.conn.peekPing() // a ping may or may not precede the subscribe
				continue
			}
			if !isSubscribe(frame) || subscribedOn == st.conn {
				continue // the SDK subscribes once per connection
			}
			got := st.conn.nextNonPing(t, "subscribe frame")
			if !jsonEqual(t, mustJSON(t, got), mustJSON(t, without(obj(frame), o.forbidden...))) {
				t.Errorf("subscribe = %v, want %v", got, without(obj(frame), o.forbidden...))
			}
			if pv, ok := got["payloadVersion"].(float64); !ok || pv != 3 {
				t.Errorf("payloadVersion = %#v, want the number 3", got["payloadVersion"])
			}
			for _, k := range append(o.forbidden, "includeContent", "includeEntities", "extended") {
				if _, ok := got[k]; ok {
					t.Errorf("subscribe carries %q", k)
				}
			}
			subscribedOn = st.conn
		case "serverSends":
			before := p.count(func() int { return p.frames })
			st.conn.send(t, step["frame"])
			waitFor(t, "client to process a frame", func() bool {
				return p.count(func() int {
					if p.frames > before || p.done {
						return 1
					}
					return 0
				}) == 1
			})
		case "serverCloses":
			if st.conn != nil && st.conn.isOpen() {
				st.conn.close()
			}
			time.Sleep(5 * time.Millisecond)
		case "advance":
			p.advance(time.Duration(step["ms"].(float64)) * time.Millisecond)
		case "expect":
			var next map[string]any
			if i+1 < len(steps) {
				next = obj(steps[i+1])
			}
			checkExpect(t, st, step, next)
		default:
			t.Fatalf("unknown step %v", step["step"])
		}
		if t.Failed() {
			break
		}
	}
	if o.after != nil && !t.Failed() {
		o.after(t, st)
	}
	p.stop()
	if n := p.clock.pending(); n != 0 {
		t.Errorf("%d timers still pending after stop", n)
	}
	waitFor(t, "all sockets closed", func() bool { return ws.open.Load() == 0 })
	waitFor(t, "stream goroutines to end", func() bool {
		closeIdle()
		return goroutines() <= baseline+1
	})
}

func checkExpect(t *testing.T, st *sessionState, step, next map[string]any) {
	t.Helper()
	p := st.p
	c := &st.checked
	switch step["outcome"] {
	case "admitted":
		waitFor(t, "admitted", func() bool { return p.count(func() int { return len(p.admitted) }) > c.admitted })
		a := p.admitted[c.admitted]
		c.admitted++
		if l := str(step["leaseId"]); l != "" && a.LeaseID != l {
			t.Errorf("leaseId = %q", a.LeaseID)
		}
		if n := str(step["clientNonce"]); n != "" && (a.ClientNonce == nil || *a.ClientNonce != n) {
			t.Errorf("clientNonce = %v", a.ClientNonce)
		}
		if a.ServerNow != 1795467845123 {
			t.Errorf("serverNow = %d", a.ServerNow)
		}
	case "noAdmitted":
		time.Sleep(10 * time.Millisecond)
		p.with(func() {
			if len(p.admitted) != c.admitted || p.terminal != nil {
				t.Errorf("admitted = %d, terminal = %v", len(p.admitted), p.terminal)
			}
		})
	case "item":
		waitFor(t, "an item", func() bool {
			return p.count(func() int {
				for _, it := range p.items[c.items:] {
					if it.Kind != StreamEventKindOther {
						return 1
					}
				}
				return 0
			}) == 1
		})
		p.with(func() {
			for p.items[c.items].Kind == StreamEventKindOther {
				c.items++
			}
			it := p.items[c.items]
			c.items++
			if string(it.Kind) != str(step["kind"]) {
				t.Errorf("kind = %s, want %s", it.Kind, step["kind"])
			}
			if id := str(step["id"]); id != "" && it.Data.ID != id {
				t.Errorf("id = %s, want %s", it.Data.ID, id)
			}
		})
	case "duplicateDropped":
		waitFor(t, "a dropped duplicate", func() bool { return p.count(func() int { return len(p.duplicates) }) > c.duplicates })
		if id := str(step["id"]); id != "" && p.duplicates[c.duplicates] != id {
			t.Errorf("duplicate id = %s", p.duplicates[c.duplicates])
		}
		c.duplicates++
	case "liveness":
		waitFor(t, "a pong refreshing the watchdog", func() bool { return p.count(func() int { return p.pongs }) > c.pongs })
		p.with(func() {
			c.pongs = p.pongs
			if p.terminal != nil || len(p.scheduled) != c.scheduled {
				t.Errorf("terminal %v, scheduled %v", p.terminal, p.scheduled)
			}
		})
	case "logOnly":
		waitFor(t, "a reported error or an other item", func() bool {
			return p.count(func() int {
				if len(p.errs) > c.errs {
					return 1
				}
				for _, it := range p.items[c.items:] {
					if it.Kind == StreamEventKindOther {
						return 1
					}
				}
				return 0
			}) == 1
		})
		time.Sleep(10 * time.Millisecond)
		p.with(func() {
			if code := str(step["code"]); code != "" {
				found := false
				for _, e := range p.errs[c.errs:] {
					var ae *APIError
					if errors.As(e, &ae) && string(ae.Code) == code {
						found = true
					}
				}
				if !found {
					t.Errorf("no reported error with code %s: %v", code, p.errs)
				}
			}
			c.errs = len(p.errs)
			for c.items < len(p.items) && p.items[c.items].Kind == StreamEventKindOther {
				c.items++
			}
			if p.terminal != nil || p.done || len(p.scheduled) != c.scheduled {
				t.Errorf("logOnly: terminal %v done %v scheduled %v", p.terminal, p.done, p.scheduled)
			}
		})
		if !st.conn.isOpen() {
			t.Error("the connection must stay open")
		}
	case "reported":
		if step["hook"] != "onQuotaWait" {
			t.Fatalf("unknown hook %v", step["hook"])
		}
		waitFor(t, "onQuotaWait", func() bool { return p.count(func() int { return len(p.quotaWaits) }) > c.quota })
		p.with(func() {
			q := p.quotaWaits[c.quota]
			c.quota++
			checkQuotaWait(t, q, step)
			if p.done || p.terminal != nil {
				t.Error("a quota wait must not end the stream")
			}
		})
	case "terminalError":
		waitFor(t, "the terminal error", func() bool { return p.isDone() })
		p.with(func() {
			checkError(t, p.terminal, step)
			if len(p.scheduled) != c.scheduled {
				t.Errorf("reconnect scheduled before the terminal error: %v", p.scheduled)
			}
		})
	case "noReconnect":
		// No dial happens now, and no reconnect is due yet.
		dials := st.ws.count.Load()
		p.advance(0)
		time.Sleep(30 * time.Millisecond)
		now := p.clock.Now()
		p.with(func() {
			for _, sc := range p.scheduled[c.scheduled:] {
				if !sc.at.Add(sc.delay).After(now) {
					t.Errorf("reconnect due: %+v", sc)
				}
			}
		})
		if st.ws.count.Load() != dials {
			t.Error("a dial happened")
		}
	case "reconnect":
		waitFor(t, "a scheduled reconnect", func() bool { return p.count(func() int { return len(p.scheduled) }) > c.scheduled })
		var s scheduled
		p.with(func() {
			s = p.scheduled[c.scheduled]
			c.scheduled++
			if p.done {
				t.Error("stream ended")
			}
		})
		switch b := step["backoff"]; b {
		case "long":
			if s.delay < 60*time.Second {
				t.Errorf("delay = %v, want a long backoff", s.delay)
			}
		case "quota":
			if s.delay < time.Hour {
				t.Errorf("delay = %v, want the quota interval", s.delay)
			}
		case true:
			if s.delay <= 0 {
				t.Errorf("delay = %v, want a backoff", s.delay)
			}
		}
		if ms, ok := step["firstRetryMinMs"].(float64); ok && s.delay < time.Duration(ms)*time.Millisecond {
			t.Errorf("delay = %v < %v ms", s.delay, ms)
		}
		if r := str(step["reason"]); r != "" && string(s.reason) != r {
			t.Errorf("reason = %s, want %s", s.reason, r)
		}
		if step["hint"] == "takeover" {
			// Hooks run on their own goroutine: wait for the report.
			waitFor(t, "the takeover hint", func() bool {
				return p.count(func() int {
					for _, e := range p.errs {
						if strings.Contains(e.Error(), "x-takeover") {
							return 1
						}
					}
					return 0
				}) == 1
			})
		}
		if rep := obj(step["reported"]); rep != nil {
			waitFor(t, "onQuotaWait", func() bool { return p.count(func() int { return len(p.quotaWaits) }) > c.quota })
			p.with(func() {
				checkQuotaWait(t, p.quotaWaits[c.quota], rep)
				c.quota++
				if len(p.errs) != c.errs {
					t.Errorf("a quota wait must not also go to onError: %v", p.errs)
				}
			})
		}
		// Not before the delay, then exactly at it.
		dials := st.ws.count.Load()
		remaining := s.at.Add(s.delay).Sub(p.clock.Now())
		if remaining > 0 {
			p.advance(remaining - time.Millisecond)
			time.Sleep(10 * time.Millisecond)
			if st.ws.count.Load() != dials {
				t.Fatalf("redial before the delay of %v", s.delay)
			}
			p.advance(time.Millisecond)
		}
		if next["step"] != "dial" {
			d := st.ws.nextDial(t, "redial")
			if step["resubscribe"] == true {
				got := d.conn.nextNonPing(t, "re-subscribe")
				if !jsonEqual(t, mustJSON(t, without(got, "clientNonce")), mustJSON(t, without(st.subscribe, "clientNonce"))) {
					t.Errorf("re-subscribe = %v, want %v", got, st.subscribe)
				}
			}
			st.conn = d.conn
		}
	default:
		t.Fatalf("unknown outcome %v", step["outcome"])
	}
}

func checkQuotaWait(t *testing.T, q QuotaWaitInfo, want map[string]any) {
	t.Helper()
	if a, ok := want["attempt"].(float64); ok && q.Attempt != int(a) {
		t.Errorf("attempt = %d", q.Attempt)
	}
	if q.NextAttemptIn != time.Hour {
		t.Errorf("nextAttemptIn = %v, want 1h", q.NextAttemptIn)
	}
	checkError(t, q.Error, want)
}

func checkError(t *testing.T, err error, want map[string]any) {
	t.Helper()
	switch want["error"] {
	case "PreemptedError":
		var pe *PreemptedError
		if !errors.As(err, &pe) || pe.Kind() != ErrorKindPreempted {
			t.Fatalf("error = %v, want *PreemptedError", err)
		}
		if string(pe.Reason) != str(want["reason"]) {
			t.Errorf("reason = %s", pe.Reason)
		}
		switch nl := want["newLeaseId"].(type) {
		case nil:
			if pe.NewLeaseID != nil {
				t.Errorf("newLeaseId = %v, want null", *pe.NewLeaseID)
			}
		case string:
			if pe.NewLeaseID == nil || *pe.NewLeaseID != nl {
				t.Errorf("newLeaseId = %v", pe.NewLeaseID)
			}
		}
	case "FinlightApiError":
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Fatalf("error = %v, want *APIError", err)
		}
		if k := str(want["apiKind"]); k != "" && string(ae.APIKind) != k {
			t.Errorf("apiKind = %s, want %s", ae.APIKind, k)
		}
		if s, ok := want["status"].(float64); ok && ae.Status != int(s) {
			t.Errorf("status = %d", ae.Status)
		}
		if code := str(want["code"]); code != "" {
			if string(ae.Code) != code {
				t.Errorf("code = %s", ae.Code)
			}
			if len(ae.Errors) == 0 || ae.Errors[0].Field == "" || ae.Errors[0].Constraints == nil {
				t.Errorf("errors = %+v, want field and constraints", ae.Errors)
			}
		}
	default:
		t.Fatalf("unknown error %v", want["error"])
	}
}

// subscribeFixture runs R-13/R-14: the exact subscribe frame and the answer.
func subscribeFixture(t *testing.T, rel string, raw bool) {
	f := loadFixture(t, rel)
	ws := newWSMock(t)
	frame := obj(f["frame"])
	p := startProbe(t, ws, probeOptions{raw: raw, params: obj(obj(f["sdk"])["args"]), apiKey: kitAPIKey, nonce: str(frame["clientNonce"])})
	defer p.stop()
	d := ws.nextDial(t, "dial")
	got := d.conn.nextNonPing(t, "subscribe")
	if !jsonEqual(t, mustJSON(t, got), mustJSON(t, frame)) {
		t.Errorf("subscribe = %s, want %s", mustJSON(t, got), mustJSON(t, frame))
	}
	var rawGot map[string]json.RawMessage
	_ = json.Unmarshal(mustJSON(t, got), &rawGot)
	if string(rawGot["payloadVersion"]) != "3" {
		t.Errorf("payloadVersion = %s", rawGot["payloadVersion"])
	}
	if _, ok := got["action"]; ok {
		t.Error("subscribe must not carry action")
	}
	frames := obj(f["server"])["frames"].([]any)
	for _, fr := range frames {
		d.conn.send(t, fr)
	}
	waitFor(t, "admitted", func() bool { return p.count(func() int { return len(p.admitted) }) == 1 })
	if !raw {
		waitFor(t, "initial push", func() bool { return p.count(func() int { return len(p.items) }) == 1 })
	} else {
		time.Sleep(10 * time.Millisecond)
		if n := p.count(func() int { return len(p.items) }); n != 0 {
			t.Errorf("raw items = %d", n)
		}
	}
	p.with(func() {
		if p.terminal != nil || len(p.errs) != 0 {
			t.Errorf("terminal %v errs %v", p.terminal, p.errs)
		}
	})
}
