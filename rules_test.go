package finlight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Tests of the client rules that the kit cases do not cover by themselves.

// R-SCOPE-1: no request type can carry a v2 key; the source has no v2 path.
func TestNoV2KeysInRequests(t *testing.T) {
	yes := true
	keys := searchWireKeys(t)
	sub, _ := StreamParams{
		Query: "q", Sources: []string{"a"}, ExcludeSources: []string{"a"}, OptInSources: []string{"a"},
		Language: "en", Tickers: []string{"X"}, Countries: []string{"US"}, Categories: []string{"c"},
		ExcludeEmptyContent: &yes, IncludeUpdates: &yes, Include: []IncludeBlock{IncludeBlockContent},
	}.subscribe()
	rawSub, _ := RawStreamParams{Query: "q", Sources: []string{"a"}, ExcludeSources: []string{"a"}, Language: "en", IncludeUpdates: &yes}.subscribe()
	all := append([]string{}, keys...)
	for k := range sub {
		all = append(all, k)
	}
	for k := range rawSub {
		all = append(all, k)
		if k == "include" {
			t.Error("raw subscribe carries include")
		}
	}
	for _, k := range all {
		for _, bad := range []string{"includeContent", "includeEntities", "extended", "pageSize", "publishDate", "isUpdate"} {
			if strings.EqualFold(k, bad) {
				t.Errorf("request key %q", k)
			}
		}
	}
}

func TestSourceHasNoV2Paths(t *testing.T) {
	bad := regexp.MustCompile(`(?i)/v1/|/v2/|sendarticle|admin_?kick|include_?content|include_?entities|page_?size|publish_?date|is_?update`)
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (p == "testdata" || p == ".git" || p == "local") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(data), "github.com/callbk/finlight-client-go/v2", "")
		for i, line := range strings.Split(text, "\n") {
			if loc := bad.FindString(line); loc != "" {
				// Only the frame decoder (R-WS-1) knows the v2 article action.
				if filepath.Base(p) == "frames.go" && strings.Contains(strings.ToLower(loc), "sendarticle") {
					continue
				}
				t.Errorf("%s:%d: %q", p, i+1, loc)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// R-LOG-1: no global handlers, no direct output.
func TestNoGlobalSideEffects(t *testing.T) {
	bad := regexp.MustCompile(`os/signal|os\.Exit|fmt\.Print|log\.Print|os\.Stdout|os\.Stderr|slog\.SetDefault`)
	for _, dir := range []string{".", "webhook"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			data, _ := os.ReadFile(f)
			var code []string
			for _, l := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(l), "//") {
					code = append(code, l)
				}
			}
			if m := bad.FindString(strings.Join(code, "\n")); m != "" {
				t.Errorf("%s uses %s", f, m)
			}
		}
	}
}

// R-TEST-1 / R-PKG-1: no library code reads the kit.
func TestLibraryDoesNotEmbedKit(t *testing.T) {
	for _, dir := range []string{".", "webhook"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			data, _ := os.ReadFile(f)
			if !strings.HasSuffix(f, "_test.go") && strings.Contains(string(data), "testdata") {
				t.Errorf("%s references testdata", f)
			}
		}
	}
}

// R-CONFIG-1
func TestConfigDefaultsAndURLs(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("an empty API key must fail")
	} else if ve := (*ValidationError)(nil); !errors.As(err, &ve) || ve.Param != "APIKey" {
		t.Fatalf("err = %v", err)
	}
	c, err := NewClient(Config{APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := c.cfg
	want := []struct {
		got, want any
	}{
		{cfg.BaseURL, "https://api.finlight.me"}, {cfg.WSSURL, "wss://wss.finlight.me"},
		{cfg.Timeout, 30 * time.Second}, {cfg.TotalTimeout, time.Duration(0)}, {cfg.MaxRetries, 3},
		{cfg.RetryBaseDelay, 500 * time.Millisecond}, {cfg.RetryMaxDelay, 8 * time.Second},
		{cfg.RetryOnBurst, BurstRetryOnce}, {cfg.BurstWait, 10 * time.Second}, {cfg.MaxPages, 10},
		{cfg.MaxResponseBytes, int64(32 << 20)}, {cfg.CompanyCacheSize, 256},
		{cfg.PingInterval, 25 * time.Second}, {cfg.WatchdogTimeout, 60 * time.Second},
		{cfg.RotationInterval, 115 * time.Minute}, {cfg.ReconnectBaseDelay, time.Second},
		{cfg.ReconnectMaxDelay, 30 * time.Second}, {cfg.LongBackoff, 60 * time.Second},
		{cfg.DisableQuotaReconnect, false}, {cfg.QuotaReconnectInterval, time.Hour},
		{cfg.SearchFailedThreshold, 5}, {cfg.DedupeCacheSize, 1000},
		{cfg.MaxMessageBytes, int64(32 << 20)}, {cfg.ConnectTimeout, 30 * time.Second},
	}
	for i, w := range want {
		if w.got != w.want {
			t.Errorf("default %d = %v, want %v", i, w.got, w.want)
		}
	}
	off, _ := NewClient(Config{APIKey: "k", MaxRetries: -1, CompanyCacheSize: -1, MaxMessageBytes: 1})
	if off.cfg.MaxRetries != 0 || off.cfg.CompanyCacheSize != 0 || off.cfg.MaxMessageBytes != 16<<20 {
		t.Errorf("off values: %+v", off.cfg.MaxRetries)
	}
	if _, err := NewClient(Config{APIKey: "k", RetryOnBurst: "twice"}); err == nil {
		t.Error("unknown RetryOnBurst accepted")
	}

	// A trailing slash does not matter.
	m := newHTTPMock(t)
	m.enqueue(mockResponse{Status: 200, Body: map[string]any{"sources": []any{}}})
	if _, err := newTestClient(t, Config{BaseURL: m.URL() + "/"}).Sources.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p := m.reqs()[0].Path; p != "/v3/sources" {
		t.Errorf("path = %s", p)
	}
	ws := newWSMock(t)
	ws.push(dialReply{status: 401, body: `{"message":"Invalid API key"}`})
	s, _ := newTestClient(t, Config{WSSURL: ws.URL() + "/"}).RawStream(t.Context(), RawStreamParams{}, nil)
	for range s.Events() {
	}
	if d := ws.nextDial(t, "dial"); d.path != "/raw" {
		t.Errorf("raw path = %s", d.path)
	}
}

// R-CONFIG-1: an injected HTTP client is used for REST and the WebSocket dial.
func TestInjectedHTTPClient(t *testing.T) {
	var calls atomic.Int32
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	m := newHTTPMock(t)
	m.enqueue(mockResponse{Status: 200, Body: map[string]any{"sources": []any{}}})
	ws := newWSMock(t)
	ws.push(dialReply{status: 401, body: `{}`})
	c := newTestClient(t, Config{BaseURL: m.URL(), WSSURL: ws.URL(), HTTPClient: hc})
	if _, err := c.Sources.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	s, _ := c.Stream(t.Context(), StreamParams{}, nil)
	for range s.Events() {
	}
	if calls.Load() != 2 {
		t.Errorf("injected client used %d times, want 2", calls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// R-LOG-1: the key never shows in logs, errors or printed values.
func TestKeyNeverLeaks(t *testing.T) {
	const key = "sk_secret_key_that_must_not_leak_0000000000000000000000000000000"
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m := newHTTPMock(t)
	m.setAlways(mockResponse{Status: 500, Body: map[string]any{"code": "INTERNAL_ERROR", "detail": "x"}})
	ws := newWSMock(t)
	ws.push(dialReply{status: 401, body: `{"message":"Invalid API key"}`})
	c := newTestClient(t, Config{APIKey: key, BaseURL: m.URL(), WSSURL: ws.URL(), Logger: logger})
	var texts []string
	_, err := c.Articles.Search(t.Context(), SearchParams{Query: "q"})
	texts = append(texts, err.Error(), fmt.Sprintf("%v %+v %#v", err, err, err))
	s, _ := c.Stream(t.Context(), StreamParams{}, nil)
	for _, err := range s.Events() {
		texts = append(texts, err.Error(), fmt.Sprintf("%+v %#v", err, err))
	}
	texts = append(texts, buf.String(), fmt.Sprintf("%v %+v %#v %s", c.cfg, c.cfg, c.cfg, c), fmt.Sprintf("%+v %#v", c, c), fmt.Sprintf("%+v", s))
	for _, txt := range texts {
		if strings.Contains(txt, key) {
			t.Fatalf("the API key leaked: %s", txt)
		}
	}
	if !strings.Contains(buf.String(), "logger=finlight") {
		t.Error("log records lack the finlight logger name")
	}
}

// R-RETRY-1: cancelling during the burst wait ends at once, unchanged.
func TestCancelDuringBurstWait(t *testing.T) {
	f := loadFixture(t, "fixtures/errors/rate-limited-burst.json")
	m := newHTTPMock(t)
	m.setAlways(fixtureResponse(f["response"]))
	clock := &stuckClock{started: make(chan time.Duration, 1)}
	c := newTestClient(t, Config{BaseURL: m.URL(), Clock: clock})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := c.Articles.Search(ctx, SearchParams{})
		done <- err
	}()
	if d := <-clock.started; d != 7*time.Second {
		t.Errorf("burst wait = %v", d)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not end the wait")
	}
	if len(m.reqs()) != 1 {
		t.Error("retried after cancel")
	}
}

func TestBurstWithoutRetryAfterWaitsBurstWait(t *testing.T) {
	m := newHTTPMock(t)
	m.setAlways(mockResponse{Status: 429, Body: map[string]any{"code": "RATE_LIMITED", "detail": "Throttling Exception: Exceeded rate limit of 5 requests per 10 seconds."}})
	clock := &instantClock{}
	_, err := newTestClient(t, Config{BaseURL: m.URL(), Clock: clock}).Articles.Get(t.Context(), "x", nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.LimitType != RateLimitBurst || len(m.reqs()) != 2 {
		t.Fatalf("err = %v, requests %d", err, len(m.reqs()))
	}
	if w := clock.sleeps(); len(w) != 1 || w[0] != 10*time.Second {
		t.Errorf("waits = %v", w)
	}
	// The detail text classifies a monthly limit without limitType.
	if classifyRateLimit("Throttling Exception: Exceeded token limit of 1000 for the current period.") != RateLimitMonthly ||
		classifyRateLimit("something else") != RateLimitUnknown {
		t.Error("detail classification")
	}
}

// R-RETRY-1: network errors and attempt timeouts are retried with the same id.
func TestNetworkErrorsRetried(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	var n atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			n.Add(1)
			_ = conn.Close()
		}
	}()
	defer ln.Close()
	clock := &instantClock{}
	_, err := newTestClient(t, Config{BaseURL: "http://" + ln.Addr().String(), Clock: clock}).Sources.List(t.Context())
	var ne *NetworkError
	if !errors.As(err, &ne) || ne.Kind() != ErrorKindNetwork {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 4 || len(clock.sleeps()) != 3 {
		t.Errorf("attempts = %d, waits = %v", n.Load(), clock.sleeps())
	}

	// An attempt timeout is a TimeoutError (also a NetworkError).
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) })
	srv := newServer(t, slow)
	_, err = newTestClient(t, Config{BaseURL: srv, Timeout: 20 * time.Millisecond, MaxRetries: -1}).Sources.List(t.Context())
	var te *TimeoutError
	if !errors.As(err, &te) || te.Kind() != ErrorKindTimeout || !errors.As(err, &ne) {
		t.Fatalf("err = %v", err)
	}
	// A total timeout bounds the whole call.
	_, err = newTestClient(t, Config{BaseURL: srv, TotalTimeout: 30 * time.Millisecond, Clock: systemClock{}}).Sources.List(t.Context())
	if !errors.As(err, &te) {
		t.Fatalf("total timeout: %v", err)
	}
}

// R-MODELS-1: a broken entry is skipped and reported; a single lookup fails.
func TestBrokenEntries(t *testing.T) {
	m := newHTTPMock(t)
	m.enqueue(mockResponse{Status: 200, Body: json.RawMessage(`{"articles":[` + article + `,{"id":"b","title":"t","source":{"domain":"d"}},{"id":"c","link":"l","title":"t","source":{"domain":"d"},"sentiment":{"label":"positive","confidence":"0.95"}}],"pagination":{"limit":3,"returned":3,"hasMore":false,"nextCursor":null}}`)})
	m.enqueue(mockResponse{Status: 200, Body: json.RawMessage(`{"article":{"id":"b","title":"t","source":{"domain":"d"}}}`)})
	var buf bytes.Buffer
	c := newTestClient(t, Config{BaseURL: m.URL(), Logger: slog.New(slog.NewTextHandler(&buf, nil))})
	page, err := c.Articles.Search(t.Context(), SearchParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Articles) != 1 || len(page.Skipped) != 2 || page.Skipped[0].Index != 1 || page.Skipped[1].Index != 2 {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(string(page.Skipped[0].Raw), `"id":"b"`) || !strings.Contains(page.Skipped[0].Reason, "link") {
		t.Errorf("skipped = %+v", page.Skipped[0])
	}
	if !strings.Contains(buf.String(), "skipped") {
		t.Error("skip not logged as a warning")
	}
	_, err = c.Articles.Get(t.Context(), "b", nil)
	var de *DecodeError
	if !errors.As(err, &de) || de.Path != "article.link" {
		t.Errorf("err = %v", err)
	}
}

const article = `{"id":"a","link":"https://x.example/1","title":"t","source":{"domain":"x.example","type":null}}`

// R-MODELS-2: times as sent, strict parsing, full number precision.
func TestTimesAndNumbers(t *testing.T) {
	a := decodeOK(t, []byte(`{"id":"a","link":"l","title":"t","source":{"domain":"d"},"summary":"","publishedAt":"now","firstSeenAt":"2026-11-23 21:04:05Z","revisedAt":"2026-11-23T21:04:05+01:00","availableAt":"1763932800","sentiment":{"label":"positive","confidence":0.912345678}}`))
	if *a.PublishedAt != "now" || *a.Summary != "" {
		t.Error("values must arrive unchanged")
	}
	if _, ok := a.PublishedAtTime(); ok {
		t.Error(`"now" parsed`)
	}
	if _, ok := a.FirstSeenAtTime(); ok {
		t.Error("space separator parsed")
	}
	if _, ok := a.AvailableAtTime(); ok {
		t.Error("unix seconds parsed")
	}
	if tm, ok := a.RevisedAtTime(); !ok || tm.UTC().Hour() != 20 {
		t.Error("offset time")
	}
	if *a.Sentiment.Value().Confidence != 0.912345678 {
		t.Error("confidence rounded")
	}
	f := decodeFrame([]byte(`{"event":"connection.admitted","payloadVersion":3,"data":{"leaseId":"l","serverNow":1763932800123,"clientNonce":null}}`))
	if f.admitted.ServerNow != 1763932800123 || f.admitted.ClientNonce != nil {
		t.Errorf("admitted = %+v", f.admitted)
	}
	// Keys match exactly: "Title" is not "title".
	var x Article
	if err := json.Unmarshal([]byte(`{"id":"a","link":"l","Title":"t","source":{"domain":"d"}}`), &x); err == nil {
		t.Error("case-insensitive key accepted")
	}
	// Wrong types fail; nothing is coerced.
	if err := json.Unmarshal([]byte(`{"id":"a","link":"l","title":"t","source":{"domain":"d"},"companies":[{"companyId":"1","name":"n"}]}`), &x); err == nil {
		t.Error(`"1" accepted as companyId`)
	}
}

// R-NULL-1: writing a model keeps absent keys absent and null as null.
func TestRoundTrip(t *testing.T) {
	for _, rel := range []string{"fixtures/decode/legacy-values.json", "fixtures/decode/raw-skeleton.json", "fixtures/decode/full-article.json", "fixtures/decode/events-without-companies.json"} {
		wire := rawFixture(t, rel)["wire"]
		out, err := json.Marshal(decodeOK(t, wire))
		if err != nil {
			t.Fatal(err)
		}
		var w map[string]any
		_ = json.Unmarshal(wire, &w)
		if _, ok := w["media"]; !ok {
			w["media"] = []any{}
		}
		if !jsonEqual(t, out, mustJSON(t, w)) {
			t.Errorf("%s: round trip\n got %s\nwant %s", rel, out, mustJSON(t, w))
		}
	}
}

// R-REQ-3: shape checks before any request or dial.
func TestLocalValidation(t *testing.T) {
	m := newHTTPMock(t)
	ws := newWSMock(t)
	c := newTestClient(t, Config{BaseURL: m.URL(), WSSURL: ws.URL()})
	ctx := t.Context()
	rel := 1.5
	checks := map[string]error{}
	_, checks["limit"] = c.Articles.Search(ctx, SearchParams{Limit: 101})
	_, checks["limit-negative"] = c.Articles.Search(ctx, SearchParams{Limit: -1})
	_, checks["minRelevance"] = c.Articles.Search(ctx, SearchParams{MinRelevance: &rel})
	_, checks["include"] = c.Articles.Search(ctx, SearchParams{Include: []IncludeBlock{"a", "b", "c", "d"}})
	_, checks["searchAll"] = c.Articles.SearchAll(ctx, SearchParams{Languages: []string{}}, nil)
	_, checks["get"] = c.Articles.Get(ctx, "", nil)
	_, checks["getByLink"] = c.Articles.GetByLink(ctx, "", nil)
	_, checks["q"] = c.Companies.Search(ctx, CompanySearchParams{Q: strings.Repeat("x", 201)})
	_, checks["mic"] = c.Companies.Search(ctx, CompanySearchParams{Q: "x", MIC: strings.Repeat("x", 33)})
	_, checks["ids"] = c.Companies.GetMany(ctx, make([]int64, 11))
	_, checks["ids-empty"] = c.Companies.GetMany(ctx, nil)
	_, checks["companyId"] = c.Companies.Get(ctx, 0, nil)
	_, checks["stream"] = c.Stream(ctx, StreamParams{Include: []IncludeBlock{"a", "b", "c", "d"}}, nil)
	for name, err := range checks {
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Kind() != ErrorKindValidation {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if len(m.reqs()) != 0 || ws.count.Load() != 0 {
		t.Error("a request or dial happened")
	}
	// An unknown vocabulary value goes out unchanged.
	m.enqueue(mockResponse{Status: 200, Body: map[string]any{"articles": []any{}, "pagination": map[string]any{}}})
	if _, err := c.Articles.Search(ctx, SearchParams{EventTypes: []EventType{"brand_new_type"}, HasContent: new(bool)}); err != nil {
		t.Fatal(err)
	}
	if string(m.reqs()[0].Body) != `{"eventTypes":["brand_new_type"],"hasContent":false}` {
		t.Errorf("body = %s", m.reqs()[0].Body)
	}
}

// R-REQ-2: formatting does not depend on the system locale.
func TestLocaleIndependent(t *testing.T) {
	for _, loc := range []string{"de_DE.UTF-8", "tr_TR.UTF-8"} {
		t.Setenv("LANG", loc)
		t.Setenv("LC_ALL", loc)
		rel := 0.5
		b, _ := SearchParams{MinRelevance: &rel, Limit: 10, Tickers: []string{"İ"}}.body()
		if got := string(marshalBody(b)); got != `{"limit":10,"minRelevance":0.5,"tickers":["İ"]}` {
			t.Errorf("%s: %s", loc, got)
		}
	}
}

// R-WS-6: hooks that panic do not end the stream; a second iteration fails;
// a cancelled context arrives unchanged; Close ends everything.
func TestStreamLifecycle(t *testing.T) {
	ws := newWSMock(t)
	c := newTestClient(t, Config{WSSURL: ws.URL(), Clock: newFakeClock()})
	ctx, cancel := context.WithCancel(t.Context())
	s, err := c.Stream(ctx, StreamParams{Query: "q"}, &StreamOptions{OnAdmitted: func(AdmittedInfo) { panic("user bug") }})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan StreamEvent, 4)
	end := make(chan error, 1)
	go func() {
		var last error
		for ev, err := range s.Events() {
			if err != nil {
				last = err
				continue
			}
			got <- ev
		}
		end <- last
	}()
	d := ws.nextDial(t, "dial")
	d.conn.nextNonPing(t, "subscribe")
	d.conn.send(t, map[string]any{"event": "connection.admitted", "payloadVersion": 3, "data": map[string]any{"leaseId": "l", "serverNow": 1, "clientNonce": nil}})
	d.conn.send(t, json.RawMessage(`{"event":"article.created","payloadVersion":3,"data":`+article+`}`))
	select {
	case ev := <-got:
		if ev.Data.ID != "a" || ev.Kind != StreamEventKindCreated {
			t.Errorf("item = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no item after a panicking hook")
	}
	for _, err := range s.Events() {
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("second iteration: %v", err)
		}
	}
	cancel()
	select {
	case err := <-end:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("end = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not end the stream")
	}
	waitFor(t, "socket closed", func() bool { return !d.conn.isOpen() })
	if !s.Closed() {
		t.Error("Closed() = false")
	}
}

func TestStreamCloseAndConcurrentStreams(t *testing.T) {
	ws := newWSMock(t)
	c := newTestClient(t, Config{WSSURL: ws.URL(), Clock: newFakeClock()})
	baseline := goroutines()
	var streams []*Stream
	var conns []*serverConn
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		s, _ := c.Stream(t.Context(), StreamParams{}, nil)
		streams = append(streams, s)
		go func() {
			for range s.Events() {
			}
			done <- struct{}{}
		}()
		conns = append(conns, ws.nextDial(t, "dial").conn)
	}
	for _, s := range streams {
		_ = s.Close()
		_ = s.Close()
	}
	<-done
	<-done
	for _, sc := range conns {
		waitFor(t, "socket closed", func() bool { return !sc.isOpen() })
	}
	waitFor(t, "goroutines to end", func() bool { closeIdle(); return goroutines() <= baseline })
}

// R-WS-2: a close with 1008 is terminal.
func TestPolicyCloseIsTerminal(t *testing.T) {
	ws := newWSMock(t)
	s, _ := newTestClient(t, Config{WSSURL: ws.URL(), Clock: newFakeClock()}).Stream(t.Context(), StreamParams{}, nil)
	end := make(chan error, 1)
	go func() {
		var last error
		for _, err := range s.Events() {
			last = err
		}
		end <- last
	}()
	d := ws.nextDial(t, "dial")
	d.conn.nextNonPing(t, "subscribe")
	_ = d.conn.c.Close(websocket.StatusPolicyViolation, "bye")
	select {
	case err := <-end:
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("1008 did not end the stream")
	}
	if ws.count.Load() != 1 {
		t.Error("redialed after 1008")
	}
}

// R-WS-2: a 403 without a readable body waits long and reports.
func TestDial403WithoutBody(t *testing.T) {
	ws := newWSMock(t)
	ws.push(dialReply{status: 403})
	clock := newFakeClock()
	var reported atomic.Int32
	s, _ := newTestClient(t, Config{WSSURL: ws.URL(), Clock: clock}).Stream(t.Context(), StreamParams{}, &StreamOptions{OnError: func(Error) { reported.Add(1) }})
	var delay atomic.Int64
	s.hooks.onScheduled = func(d time.Duration, r ReconnectReason) { delay.Store(int64(d)) }
	go func() {
		for range s.Events() {
		}
	}()
	defer s.Close()
	waitFor(t, "scheduled", func() bool { return delay.Load() != 0 })
	if time.Duration(delay.Load()) < 60*time.Second || reported.Load() != 1 {
		t.Errorf("delay = %v, reported = %d", time.Duration(delay.Load()), reported.Load())
	}
}

// R-WS-6: a raw stream passes firstSeenAt to OnReconnect.
func TestRawReconnectPassesFirstSeenAt(t *testing.T) {
	ws := newWSMock(t)
	clock := newFakeClock()
	recon := make(chan ReconnectInfo, 1)
	s, _ := newTestClient(t, Config{WSSURL: ws.URL(), Clock: clock}).RawStream(t.Context(), RawStreamParams{}, &StreamOptions{OnReconnect: func(r ReconnectInfo) { recon <- r }})
	var sched atomic.Int64
	s.hooks.onScheduled = func(d time.Duration, r ReconnectReason) { sched.Store(int64(d) + 1) }
	go func() {
		for range s.Events() {
		}
	}()
	defer s.Close()
	d := ws.nextDial(t, "dial")
	d.conn.nextNonPing(t, "subscribe")
	d.conn.send(t, json.RawMessage(`{"event":"article.created","payloadVersion":3,"data":{"id":"a","link":"l","title":"t","source":{"domain":"d"},"firstSeenAt":"2026-11-23T21:00:00.000Z","availableAt":null}}`))
	waitFor(t, "last seen", func() bool { return s.LastSeenAt() != nil })
	d.conn.close()
	waitFor(t, "scheduled", func() bool { return sched.Load() != 0 })
	clock.Advance(time.Duration(sched.Load()-1), nil)
	select {
	case r := <-recon:
		if r.LastSeenAt == nil || *r.LastSeenAt != "2026-11-23T21:00:00.000Z" || r.Reason != ReconnectClosed || r.Connection != 2 {
			t.Errorf("reconnect = %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reconnect")
	}
}

func newServer(t *testing.T, h http.Handler) string {
	t.Helper()
	m := newHTTPMock(t)
	m.srv.Config.Handler = h
	return m.URL()
}

// R-ERR-1: the NestJS fallback with a list of messages, and a text body.
func TestErrorBodyForms(t *testing.T) {
	e := parseAPIError(400, http.Header{}, []byte(`{"statusCode":400,"message":["limit must be a number","order must be asc or desc"],"error":"Bad Request","extra":1}`), "req-x")
	if e.APIKind != APIKindValidationFailed || e.Code != "" || len(e.Errors) != 2 || e.RequestID != "req-x" || !strings.Contains(e.Detail, "order must") {
		t.Errorf("nestjs list = %+v", e)
	}
	e = parseAPIError(503, http.Header{}, []byte("upstream connect error"), "req-y")
	if e.APIKind != APIKindOther || e.Detail != "upstream connect error" || e.RawJSON() != nil || e.RequestID != "req-y" {
		t.Errorf("text body = %+v", e)
	}
	e = parseAPIError(429, http.Header{"Retry-After": {"3"}}, []byte(`{"code":"RATE_LIMITED","detail":"Throttling Exception: Exceeded token limit of 1000 for the current period.","future":{"x":1}}`), "r")
	if e.LimitType != RateLimitMonthly || e.RetryAfter != 3*time.Second || e.APIKind != APIKindRateLimited {
		t.Errorf("429 = %+v", e)
	}
}
