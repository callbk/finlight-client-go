package finlight

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// ---- V: vocabulary ----

var _ = conformance("V-01", func(t *testing.T) {
	v := loadFixture(t, "vocab.json")
	list := func(path ...string) []string {
		var cur any = v
		for _, p := range path {
			cur = obj(cur)[p]
		}
		var out []string
		for _, x := range cur.([]any) {
			if m, ok := x.(map[string]any); ok {
				out = append(out, str(m["code"]))
			} else {
				out = append(out, str(x))
			}
		}
		return out
	}
	check := func(name string, got, want []string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, vocab.json has %v", name, got, want)
		}
	}
	check("IncludeBlock", toStrings(IncludeBlockValues()), list("request", "include", "values"))
	check("OrderBy", toStrings(OrderByValues()), list("request", "orderBy", "values"))
	check("SortOrder", toStrings(SortOrderValues()), list("request", "order", "values"))
	check("CompanyRole (request)", toStrings(CompanyRoleValues()), list("request", "roles", "values"))
	check("CompanyRole", toStrings(CompanyRoleValues()), list("response", "roles", "values"))
	check("EventType (request)", toStrings(EventTypeValues()), list("request", "eventTypes", "values"))
	check("EventType", toStrings(EventTypeValues()), list("response", "eventTypes", "values"))
	check("EventDirection", toStrings(EventDirectionValues()), list("response", "eventDirections", "values"))
	check("Importance (request)", toStrings(ImportanceValues()), list("request", "minImportance", "values"))
	check("Importance", toStrings(ImportanceValues()), list("response", "importance", "values"))
	check("SentimentLabel", toStrings(SentimentLabelValues()), list("response", "sentimentLabels", "values"))
	check("SourceType (request)", toStrings(SourceTypeValues()), list("request", "sourceTypes", "values"))
	check("SourceType (request exclude)", toStrings(SourceTypeValues()), list("request", "excludeSourceTypes", "values"))
	check("SourceType", toStrings(SourceTypeValues()), list("response", "sourceTypes", "values"))
	check("StreamEventName", toStrings(StreamEventNameValues()), list("websocket", "events", "values"))
	check("StreamEventName articleEvents", toStrings(StreamEventNameArticleEvents()), list("websocket", "articleEvents", "values"))
	check("StreamEventName connectionEvents", toStrings(StreamEventNameConnectionEvents()), list("websocket", "connectionEvents", "values"))
	check("ConnectionErrorCode", toStrings(ConnectionErrorCodeValues()), list("websocket", "connectionErrorCodes", "values"))
	check("PreemptReason", toStrings(PreemptReasonValues()), list("websocket", "preemptReasons", "values"))
	check("WebhookEventName", toStrings(WebhookEventNameValues()), list("webhook", "events", "values"))
	check("ProblemCode", toStrings(ProblemCodeValues()), list("problemCodes", "values"))
	check("RateLimitType", toStrings(RateLimitTypeValues()), append(list("limitTypes", "values"), "unknown"))
	// Not in vocab.json; the list is the one in sdk-style (stream section).
	check("ReconnectReason", toStrings(ReconnectReasonValues()), []string{
		"rotation", "watchdog", "closed", "handshake_failed", "search_failed",
		"dial_failed", "dial_refused", "connection_limit", "rate_limited", "quota_exceeded",
	})
	if PayloadVersion != int(v["payloadVersion"].(float64)) {
		t.Errorf("PayloadVersion = %d", PayloadVersion)
	}
	// Problem types follow the documented URI scheme.
	for _, p := range v["problemCodes"].(map[string]any)["values"].([]any) {
		m := obj(p)
		if want := "https://docs.finlight.me/errors/" + kebab(str(m["code"])); str(m["type"]) != want {
			t.Errorf("type of %s = %s", m["code"], m["type"])
		}
	}
	// Response enums stay open: an unknown value is kept.
	a := decodeOK(t, []byte(`{"id":"x","link":"l","title":"t","source":{"domain":"d","type":"brand_new"},"sentiment":{"label":"mixed","confidence":1}}`))
	if string(*a.Source.Type) != "brand_new" || string(*a.Sentiment.Value().Label) != "mixed" {
		t.Errorf("unknown enum values not kept")
	}
	// Model constants and vocab are open strings: comparing works on the string.
	if CompanyRoleAcquirer != CompanyRole("acquirer") {
		t.Error("constant mismatch")
	}
})

func kebab(code string) string {
	out := []byte{}
	for _, c := range []byte(code) {
		switch {
		case c == '_':
			out = append(out, '-')
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func decodeOK(t testing.TB, raw []byte) *Article {
	t.Helper()
	var a Article
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return &a
}

// ---- D: decode ----

// presence reads the state of a key path through the model's public API.
func presence(t testing.TB, a *Article, path string) string {
	t.Helper()
	ptr := func(isNil bool) string {
		if isNil {
			return "null"
		}
		return "value"
	}
	list := func(n int) string {
		if n == 0 {
			return "empty"
		}
		return "value"
	}
	field := func(present, isNull bool, n int) string {
		switch {
		case !present:
			return "absent"
		case isNull:
			return "null"
		case n == 0:
			return "empty"
		}
		return "value"
	}
	switch path {
	case "id", "link", "title", "source", "source.domain":
		return "value"
	case "summary":
		return ptr(a.Summary == nil)
	case "language":
		return ptr(a.Language == nil)
	case "publishedAt":
		return ptr(a.PublishedAt == nil)
	case "firstSeenAt":
		return ptr(a.FirstSeenAt == nil)
	case "revisedAt":
		return ptr(a.RevisedAt == nil)
	case "availableAt":
		return ptr(a.AvailableAt == nil)
	case "media":
		return list(len(a.Media))
	case "countries":
		return list(len(a.Countries))
	case "categories":
		return list(len(a.Categories))
	case "source.type":
		return ptr(a.Source.Type == nil)
	case "content":
		return field(a.ContentPresent(), a.Content.IsNull(), 1)
	case "sentiment":
		return field(a.SentimentPresent(), a.Sentiment.IsNull(), 1)
	case "companies":
		return field(a.CompaniesPresent(), a.Companies.IsNull(), len(a.Companies.Value()))
	case "events":
		return field(a.EventsPresent(), a.Events.IsNull(), len(a.Events.Value()))
	case "sentiment.label":
		return ptr(a.Sentiment.Value().Label == nil)
	case "sentiment.confidence":
		return ptr(a.Sentiment.Value().Confidence == nil)
	}
	m := regexp.MustCompile(`^(companies|events)\[(\d+)\]\.(.+)$`).FindStringSubmatch(path)
	if m == nil {
		t.Fatalf("no presence mapping for %s", path)
	}
	i, _ := strconv.Atoi(m[2])
	if m[1] == "companies" {
		c := a.Companies.Value()[i]
		switch m[3] {
		case "companyId", "name":
			return "value"
		case "isin":
			return ptr(c.ISIN == nil)
		case "primaryListing":
			return ptr(c.PrimaryListing == nil)
		case "confidence":
			return ptr(c.Confidence == nil)
		case "role":
			return ptr(c.Role == nil)
		case "relevance":
			return ptr(c.Relevance == nil)
		case "primaryListing.ticker":
			return "value"
		case "primaryListing.mic":
			return ptr(c.PrimaryListing.MIC == nil)
		case "primaryListing.country":
			return ptr(c.PrimaryListing.Country == nil)
		}
	} else {
		e := a.Events.Value()[i]
		switch m[3] {
		case "type":
			return "value"
		case "direction":
			return ptr(e.Direction == nil)
		case "importance":
			return ptr(e.Importance == nil)
		case "confidence":
			return ptr(e.Confidence == nil)
		case "companyIds":
			return field(e.CompanyIDs.Present(), e.CompanyIDs.IsNull(), len(e.CompanyIDs.Value()))
		}
	}
	t.Fatalf("no presence mapping for %s", path)
	return ""
}

func checkPresence(t *testing.T, a *Article, expect any) {
	t.Helper()
	for path, want := range obj(obj(expect)["presence"]) {
		if got := presence(t, a, path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}

func decodeCase(t *testing.T, rel string) (*Article, map[string]any) {
	t.Helper()
	raw := rawFixture(t, rel)
	f := loadFixture(t, rel)
	a := decodeOK(t, raw["wire"])
	checkPresence(t, a, f["expect"])
	// Wire values arrive unchanged.
	wire := obj(f["wire"])
	if a.ID != wire["id"] || a.Link != wire["link"] || a.Title != wire["title"] {
		t.Errorf("id/link/title changed")
	}
	if !slices.Equal(a.Countries, anyStrings(wire["countries"])) || !slices.Equal(a.Categories, anyStrings(wire["categories"])) {
		t.Errorf("countries/categories changed")
	}
	if string(a.RawJSON()) != string(raw["wire"]) {
		t.Errorf("RawJSON differs from the wire text")
	}
	return a, wire
}

func anyStrings(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		out = append(out, str(x))
	}
	return out
}

var _ = conformance("D-01", func(t *testing.T) {
	a, wire := decodeCase(t, "fixtures/decode/full-article.json")
	if len(a.Media) == 0 || len(a.Media) > 3 {
		t.Errorf("media = %d entries", len(a.Media))
	}
	if a.RevisedAt == nil || *a.RevisedAt != wire["revisedAt"] {
		t.Errorf("revisedAt = %v", a.RevisedAt)
	}
	cs := a.Companies.Value()
	if cs[1].PrimaryListing != nil || cs[0].PrimaryListing == nil || *cs[0].PrimaryListing.MIC != "XNAS" {
		t.Errorf("primaryListing wrong: %+v", cs)
	}
	if cs[0].CompanyID != 900001 || *cs[0].Role != CompanyRoleAcquirer || *cs[0].Relevance != 0.95 {
		t.Errorf("company 0 = %+v", cs[0])
	}
	ev := a.Events.Value()[0]
	if ev.Type != EventTypeMNA || !slices.Equal(ev.CompanyIDs.Value(), []int64{900001, 900002}) {
		t.Errorf("event = %+v", ev)
	}
	if c, ok := a.Content.Get(); !ok || c != wire["content"] {
		t.Errorf("content = %q", c)
	}
	if s, ok := a.Sentiment.Get(); !ok || *s.Confidence != 0.9812 || *s.Label != SentimentLabelPositive {
		t.Errorf("sentiment = %+v", s)
	}
	if _, ok := a.PublishedAtTime(); !ok {
		t.Error("publishedAt does not parse")
	}
})

var _ = conformance("D-02", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/raw-skeleton.json")
	if a.ContentPresent() || a.SentimentPresent() || a.CompaniesPresent() || a.EventsPresent() {
		t.Error("gated keys must be absent on raw")
	}
	if a.AvailableAt != nil {
		t.Error("availableAt must be null on raw")
	}
})

var _ = conformance("D-03", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/no-include-full-access.json")
	if !a.SentimentPresent() || a.ContentPresent() || a.CompaniesPresent() || a.EventsPresent() {
		t.Error("only sentiment may be present")
	}
})

var _ = conformance("D-04", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/all-include-no-access.json")
	if a.ContentPresent() || a.SentimentPresent() || a.CompaniesPresent() || a.EventsPresent() {
		t.Error("gated keys must be absent")
	}
	if a.Companies.Value() != nil || a.Events.Value() != nil {
		t.Error("absent lists must not read as []")
	}
	// Writing the model back keeps absent keys absent.
	out, _ := json.Marshal(a)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(out, &m)
	for _, k := range []string{"content", "sentiment", "companies", "events"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s written although absent", k)
		}
	}
})

var _ = conformance("D-05", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/events-without-companies.json")
	ev := a.Events.Value()[0]
	if ev.CompanyIDs.Present() || ev.CompanyIDs.Value() != nil {
		t.Error("companyIds must be absent (the language null), never []")
	}
})

var _ = conformance("D-06", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/events-companies-not-entitled.json")
	if a.CompaniesPresent() || !a.EventsPresent() || a.Events.Value()[0].CompanyIDs.Present() {
		t.Error("companies absent, events present without companyIds")
	}
})

var _ = conformance("D-07", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/legacy-values.json")
	if a.PublishedAt == nil || *a.PublishedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("publishedAt = %v", a.PublishedAt)
	}
	if tm, ok := a.PublishedAtTime(); !ok || tm.Unix() != 0 {
		t.Errorf("publishedAt parses to %v", tm)
	}
	if a.Summary != nil || a.FirstSeenAt != nil || a.RevisedAt != nil || a.AvailableAt != nil {
		t.Error("legacy nulls changed")
	}
	if !a.Sentiment.IsNull() || !a.Content.IsNull() {
		t.Error("sentiment and content must be null")
	}
	c := a.Companies.Value()[0]
	if c.Role != nil || c.Relevance != nil || c.PrimaryListing == nil || c.PrimaryListing.Ticker != "LGCY" {
		t.Errorf("company = %+v", c)
	}
	if evs, ok := a.Events.Get(); !ok || len(evs) != 0 || evs == nil {
		t.Error("events must be [] (present, empty)")
	}
	if a.Countries == nil || a.Categories == nil || a.Media == nil {
		t.Error("lists must read as []")
	}
})

var _ = conformance("D-08", func(t *testing.T) {
	a, wire := decodeCase(t, "fixtures/decode/non-ascii.json")
	if a.Title != wire["title"] || *a.Summary != wire["summary"] || a.Content.Value() != wire["content"] {
		t.Error("UTF-8 text changed")
	}
})

var _ = conformance("D-09", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/partial-values.json")
	s := a.Sentiment.Value()
	if s.Label != nil || s.Confidence == nil || *s.Confidence != 0.42 {
		t.Errorf("sentiment = %+v", s)
	}
	if cs, ok := a.Companies.Get(); !ok || cs == nil || len(cs) != 0 {
		t.Error("companies must be [] (tagged, no company), not absent")
	}
	e := a.Events.Value()[0]
	if e.Direction != nil || e.Importance != nil || e.Confidence != nil {
		t.Errorf("event = %+v", e)
	}
})

var _ = conformance("D-10", func(t *testing.T) {
	f := rawFixture(t, "fixtures/decode/matrix.json")
	var cases []struct {
		ID      string          `json:"id"`
		Channel string          `json:"channel"`
		Wire    json.RawMessage `json:"wire"`
		Expect  any             `json:"expect"`
	}
	if err := json.Unmarshal(f["cases"], &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 256 {
		t.Fatalf("matrix has %d sub-cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			a := decodeOK(t, c.Wire)
			checkPresence(t, a, c.Expect)
			if c.Channel == "raw" && a.AvailableAt != nil {
				t.Error("availableAt must be null on raw")
			}
		})
	}
})

var _ = conformance("D-11", func(t *testing.T) {
	a, _ := decodeCase(t, "fixtures/decode/blank-text.json")
	if a.Summary != nil || !a.Content.IsNull() {
		t.Error("blank summary and content must read as null")
	}
	if a.Title == "" {
		t.Error("title affected")
	}
})

// ---- F: forward compatibility ----

func canonicalArticle(t testing.TB, raw json.RawMessage) string {
	t.Helper()
	a := decodeOK(t, raw)
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var _ = conformance("F-01", func(t *testing.T) {
	f := rawFixture(t, "fixtures/forward-compat/unknown-fields.json")
	if got, want := canonicalArticle(t, f["wire"]), canonicalArticle(t, f["known"]); got != want {
		t.Errorf("known fields differ:\n got %s\nwant %s", got, want)
	}
	a := decodeOK(t, f["wire"])
	var raw map[string]any
	_ = json.Unmarshal(a.RawJSON(), &raw)
	if raw["wordCount"] != float64(512) || obj(obj(raw["sentiment"]))["score"] != 0.73 {
		t.Error("unknown fields not reachable through RawJSON")
	}
})

var _ = conformance("F-02", func(t *testing.T) {
	f := rawFixture(t, "fixtures/forward-compat/unknown-enum-values.json")
	a := decodeOK(t, f["wire"])
	c := a.Companies.Value()[0]
	e := a.Events.Value()[0]
	got := []string{
		string(*a.Source.Type), string(*a.Sentiment.Value().Label), string(*c.Role),
		string(e.Type), string(*e.Direction), string(*e.Importance), a.Categories[1],
	}
	want := []string{"wire_service", "mixed", "guarantor", "esg_controversy", "mixed", "extreme", "synthetic-new-category"}
	if !slices.Equal(got, want) {
		t.Errorf("open enum values = %v, want %v", got, want)
	}
})

var _ = conformance("F-03", func(t *testing.T) {
	f := rawFixture(t, "fixtures/forward-compat/envelope-keys.json")
	var envs []struct {
		Kind string          `json:"kind"`
		Wire json.RawMessage `json:"wire"`
	}
	if err := json.Unmarshal(f["envelopes"], &envs); err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		t.Run(e.Kind, func(t *testing.T) {
			switch e.Kind {
			case "rest.search":
				m := newHTTPMock(t)
				m.enqueue(mockResponse{Status: 200, Body: e.Wire})
				page, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Search(t.Context(), SearchParams{})
				if err != nil || len(page.Articles) != 1 || page.Pagination.Limit != 1 || page.Pagination.HasMore {
					t.Fatalf("search page = %+v, %v", page, err)
				}
			case "rest.article":
				m := newHTTPMock(t)
				m.enqueue(mockResponse{Status: 200, Body: e.Wire})
				a, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Get(t.Context(), "EU8JwpsZ8iJehuyPZjMqRz", nil)
				if err != nil || a.ID != "EU8JwpsZ8iJehuyPZjMqRz" {
					t.Fatalf("article = %+v, %v", a, err)
				}
			case "webhook":
				var ev WebhookEvent
				if err := json.Unmarshal(e.Wire, &ev); err != nil || ev.EventID == "" || ev.Data.ID == "" {
					t.Fatalf("webhook = %+v, %v", ev, err)
				}
			case "ws.article":
				fr := decodeFrame(e.Wire)
				if fr.kind != frameArticle || fr.article.ID != "EU8JwpsZ8iJehuyPZjMqRz" {
					t.Fatalf("frame = %+v", fr)
				}
			case "ws.control":
				if fr := decodeFrame(e.Wire); fr.kind != framePong {
					t.Fatalf("frame kind = %v", fr.kind)
				}
			default:
				t.Fatalf("unknown envelope kind %s", e.Kind)
			}
		})
	}
})

var _ = conformance("F-04", func(t *testing.T) {
	f := rawFixture(t, "fixtures/forward-compat/unknown-events-and-codes.json")
	var items []struct {
		Kind    string            `json:"kind"`
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Wire    json.RawMessage   `json:"wire"`
	}
	if err := json.Unmarshal(f["items"], &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		t.Run(it.Kind, func(t *testing.T) {
			switch it.Kind {
			case "webhook.event":
				var ev WebhookEvent
				if err := json.Unmarshal(it.Wire, &ev); err != nil || ev.Event != "article.retracted" {
					t.Fatalf("event = %q, %v", ev.Event, err)
				}
			case "problem.code", "problem.limitType":
				m := newHTTPMock(t)
				m.setAlways(mockResponse{Status: it.Status, Header: it.Headers, Body: it.Wire})
				clock := &instantClock{}
				_, err := newTestClient(t, Config{BaseURL: m.URL(), Clock: clock}).Articles.Search(t.Context(), SearchParams{})
				var ae *APIError
				if !errors.As(err, &ae) {
					t.Fatalf("err = %v", err)
				}
				if ae.Status != it.Status || ae.RequestID != "req_synthetic" || ae.Detail == "" {
					t.Errorf("error = %+v", ae)
				}
				if it.Kind == "problem.code" && (ae.Code != "PAYMENT_REQUIRED" || ae.APIKind != APIKindOther) {
					t.Errorf("code %s kind %s", ae.Code, ae.APIKind)
				}
				if it.Kind == "problem.limitType" && (ae.LimitType != "daily" || ae.APIKind != APIKindRateLimited) {
					t.Errorf("limitType %s kind %s", ae.LimitType, ae.APIKind)
				}
				if n := len(m.reqs()); n != 1 || len(clock.sleeps()) != 0 {
					t.Errorf("attempts = %d, waits = %v; want no retry", n, clock.sleeps())
				}
			default:
				t.Fatalf("unknown item %s", it.Kind)
			}
		})
	}
})

var _ = fmt.Sprint
