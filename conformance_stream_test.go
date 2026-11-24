package finlight

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func goroutines() int { return runtime.NumGoroutine() }

// closeIdle drops idle keep-alive connections of the default transport (a
// refused dial is a plain HTTP response whose connection goes back to the pool).
func closeIdle() { http.DefaultTransport.(*http.Transport).CloseIdleConnections() }

var _ = conformance("R-13", func(t *testing.T) { subscribeFixture(t, "fixtures/requests/ws-subscribe-enriched.json", false) })
var _ = conformance("R-14", func(t *testing.T) { subscribeFixture(t, "fixtures/requests/ws-subscribe-raw.json", true) })

func session(id, file string, o sessionOptions) bool {
	return conformance(id, func(t *testing.T) { runSession(t, "ws-sessions/"+file, o) })
}

var noAutoPong = false

var (
	_ = session("S-01", "v2-pong-before-subscribe.json", sessionOptions{after: func(t *testing.T, s *sessionState) {
		// A v2 pong is the same liveness signal: send one before any ping.
		before := s.p.count(func() int { return s.p.pongs })
		s.conn.send(t, map[string]any{"action": "pong", "t": 3, "serverNow": 1795467845123})
		waitFor(t, "v2 pong", func() bool { return s.p.count(func() int { return s.p.pongs }) > before })
	}})
	_ = session("S-03", "admitted-nonce.json", sessionOptions{})
	_ = session("S-04", "created-then-updated.json", sessionOptions{})
	_ = session("S-05", "unknown-event.json", sessionOptions{after: func(t *testing.T, s *sessionState) {
		s.p.with(func() {
			if len(s.p.items) == 0 || s.p.items[0].Kind != StreamEventKindOther || s.p.items[0].Event != "connection.draining" || s.p.items[0].Data != nil {
				t.Errorf("unknown event must arrive as kind other: %+v", s.p.items)
			}
			if !strings.Contains(string(s.p.items[0].RawJSON()), "inMs") {
				t.Error("the raw frame must be reachable")
			}
		})
	}})
	_ = session("S-06", "ping-on-default-route.json", sessionOptions{})
	_ = session("S-07", "validation-failed-forbidden-key.json", sessionOptions{forbidden: []string{"includeContent"}})
	_ = session("S-08", "raw-include-rejected.json", sessionOptions{forbidden: []string{"include"}})
	_ = session("S-09", "preempted-connection-limit.json", sessionOptions{})
	_ = session("S-10", "preempted-quota-exceeded.json", sessionOptions{})
	_ = session("S-11", "v2-preempt-before-subscribe.json", sessionOptions{})
	_ = session("S-12", "v2-invalid-json-error.json", sessionOptions{})
	_ = session("S-13", "handshake-failed.json", sessionOptions{})
	_ = session("S-14", "search-failed.json", sessionOptions{after: searchFailedSurfaces})
	_ = session("S-15", "internal-error.json", sessionOptions{})
	_ = session("S-16", "pong-watchdog.json", sessionOptions{autoPong: &noAutoPong})
	_ = session("S-17", "raw-stream.json", sessionOptions{after: func(t *testing.T, s *sessionState) {
		s.p.with(func() {
			a := s.p.items[0].Data
			if a.ContentPresent() || a.SentimentPresent() || a.CompaniesPresent() || a.EventsPresent() || a.AvailableAt != nil {
				t.Errorf("raw item = %+v", a)
			}
		})
		// A raw stream reports firstSeenAt as the last seen time.
		if ls := s.p.stream.LastSeenAt(); ls == nil || *ls != *s.p.items[0].Data.FirstSeenAt {
			t.Errorf("LastSeenAt = %v", ls)
		}
	}})
	_ = session("S-18", "rotation.json", sessionOptions{after: dedupeSurvivesRotation})
	_ = session("S-19", "dial-missing-key.json", sessionOptions{})
	_ = session("S-20", "dial-invalid-key.json", sessionOptions{})
	_ = session("S-21", "dial-malformed-key.json", sessionOptions{})
	_ = session("S-22", "dial-no-websockets.json", sessionOptions{})
	_ = session("S-23", "dial-too-many-connections.json", sessionOptions{})
	_ = session("S-24", "dial-quota-exceeded.json", sessionOptions{after: quotaRedialsHourly})
	_ = session("S-25", "dial-admit-race.json", sessionOptions{})
	_ = session("S-26", "dial-server-error.json", sessionOptions{})
	_ = session("S-27", "preempted-quota-exceeded-no-reconnect.json", sessionOptions{})
	_ = session("S-28", "dial-quota-exceeded-no-reconnect.json", sessionOptions{})
)

// searchFailedSurfaces: SEARCH_FAILED reaches onError only after 5 in a row.
func searchFailedSurfaces(t *testing.T, s *sessionState) {
	p := s.p
	for i := 2; i <= 5; i++ {
		if n := p.count(func() int { return len(p.errs) }); n != 0 {
			t.Fatalf("error surfaced after %d failures", i-1)
		}
		s.conn.send(t, map[string]any{"event": "connection.error", "payloadVersion": 3, "data": map[string]any{"code": "SEARCH_FAILED", "message": "x"}})
		waitFor(t, "scheduled reconnect", func() bool { return p.count(func() int { return len(p.scheduled) }) >= i })
		sc := p.scheduled[i-1]
		if sc.reason != ReconnectReasonSearchFailed {
			t.Errorf("reason = %s", sc.reason)
		}
		p.advance(sc.at.Add(sc.delay).Sub(p.clock.Now()))
		s.conn = s.ws.nextDial(t, "redial").conn
		s.conn.nextNonPing(t, "re-subscribe")
	}
	waitFor(t, "the surfaced error", func() bool { return p.count(func() int { return len(p.errs) }) == 1 })
	var ae *APIError
	if !errors.As(p.errs[0], &ae) || ae.Code != "SEARCH_FAILED" {
		t.Errorf("surfaced error = %v", p.errs[0])
	}
	p.with(func() {
		if len(p.reconnects) != 5 || p.reconnects[4].Reason != ReconnectReasonSearchFailed || p.reconnects[4].Connection != 6 {
			t.Errorf("reconnects = %+v", p.reconnects)
		}
	})
}

// dedupeSurvivesRotation: the replayed initial push after a rotation is not
// delivered twice.
func dedupeSurvivesRotation(t *testing.T, s *sessionState) {
	p := s.p
	frame := loadFixture(t, "ws-sessions/created-then-updated.json")["steps"].([]any)[4]
	article := obj(frame)["frame"]
	s.conn.send(t, article)
	waitFor(t, "item", func() bool { return p.count(func() int { return len(p.items) }) == 1 })
	p.advance(rotationForTest)
	d := s.ws.nextDial(t, "rotation dial")
	d.conn.nextNonPing(t, "subscribe")
	waitFor(t, "old connection closed", func() bool { return !s.conn.isOpen() })
	d.conn.send(t, article)
	waitFor(t, "duplicate dropped", func() bool { return p.count(func() int { return len(p.duplicates) }) == 1 })
	waitFor(t, "onReconnect", func() bool { return p.count(func() int { return len(p.reconnects) }) == 2 })
	p.with(func() {
		if len(p.items) != 1 {
			t.Errorf("items = %d after the replay", len(p.items))
		}
		if len(p.reconnects) != 2 || p.reconnects[1].Reason != ReconnectReasonRotation || p.reconnects[1].LastSeenAt == nil {
			t.Errorf("reconnects = %+v", p.reconnects)
		}
	})
}

const rotationForTest = 115 * time.Minute

// quotaRedialsHourly: every refused attempt is reported the same way, and the
// next attempt again waits the full interval.
func quotaRedialsHourly(t *testing.T, s *sessionState) {
	p := s.p
	s.conn.send(t, map[string]any{"event": "connection.preempted", "payloadVersion": 3,
		"data": map[string]any{"reason": "quota_exceeded", "newLeaseId": nil, "timestamp": 1795467845123}})
	waitFor(t, "second quota wait", func() bool { return p.count(func() int { return len(p.quotaWaits) }) == 2 })
	p.with(func() {
		q := p.quotaWaits[1]
		var pe *PreemptedError
		if q.Attempt != 2 || q.NextAttemptIn != time.Hour || !errors.As(q.Error, &pe) {
			t.Errorf("second quota wait = %+v", q)
		}
		if len(p.errs) != 0 {
			t.Errorf("quota waits must not reach onError: %v", p.errs)
		}
	})
	dials := s.ws.count.Load()
	p.advance(time.Hour - time.Second)
	time.Sleep(10 * time.Millisecond)
	if s.ws.count.Load() != dials {
		t.Error("redial before the hour")
	}
	p.advance(time.Second)
	s.ws.nextDial(t, "hourly redial").conn.nextNonPing(t, "re-subscribe")
}

// S-02: a raw TCP listener reads the handshake bytes as they are on the wire.
var _ = conformance("S-02", func(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			got <- ""
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		var b strings.Builder
		for {
			line, err := r.ReadString('\n')
			b.WriteString(line)
			if err != nil || line == "\r\n" {
				break
			}
		}
		got <- b.String()
		body := `{"message":"Invalid API key"}`
		_, _ = io.WriteString(conn, "HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json\r\nContent-Length: "+strconv.Itoa(len(body))+"\r\nConnection: close\r\n\r\n"+body)
	}()
	c := newTestClient(t, Config{WSSURL: "ws://" + ln.Addr().String()})
	s, err := c.Stream(t.Context(), StreamParams{Query: "ticker:XMPL"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var terminal error
	for _, err := range s.Events() {
		terminal = err
	}
	raw := <-got
	reqLine, _, _ := strings.Cut(raw, "\r\n")
	if strings.Contains(strings.ToLower(reqLine), "apikey") {
		t.Errorf("the key is in the request line: %s", reqLine)
	}
	if !strings.Contains(raw, "\r\nx-api-key: "+kitAPIKey+"\r\n") {
		t.Errorf("handshake lacks a lowercase x-api-key header:\n%s", strings.ReplaceAll(raw, kitAPIKey, "<key>"))
	}
	if strings.Contains(raw, "X-Api-Key") || strings.Contains(raw, "X-API-KEY") {
		t.Error("the key header was canonicalized")
	}
	if !strings.Contains(raw, "\r\nx-client-version: "+Version+"\r\n") || !strings.Contains(raw, "\r\nUser-Agent: finlight-client-go/"+Version+"\r\n") {
		t.Errorf("version headers missing:\n%s", strings.ReplaceAll(raw, kitAPIKey, "<key>"))
	}
	var ae *APIError
	if !errors.As(terminal, &ae) || ae.Status != 401 || ae.APIKind != APIKindUnauthorized {
		t.Errorf("terminal = %v", terminal)
	}
})
