package finlight

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// expectRequest compares method, path, decoded query, parsed JSON body and the
// API key header of a recorded request with a fixture "request".
func expectRequest(t *testing.T, got recordedRequest, want any) {
	t.Helper()
	w := obj(want)
	if got.Method != w["method"] || got.Path != w["path"] {
		t.Errorf("request = %s %s, want %s %s", got.Method, got.Path, w["method"], w["path"])
	}
	if !slices.Equal(queryPairs(t, got.RawQuery), queryPairs(t, str(w["query"]))) {
		t.Errorf("query = %q, want %q", got.RawQuery, w["query"])
	}
	if strings.Contains(got.RawQuery, "%5B%5D") || strings.Contains(got.RawQuery, "[]") {
		t.Errorf("query uses include[]= form: %s", got.RawQuery)
	}
	if body, ok := w["body"]; ok {
		if !jsonEqual(t, got.Body, mustJSON(t, body)) {
			t.Errorf("body = %s, want %s", got.Body, mustJSON(t, body))
		}
		if !strings.HasPrefix(got.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type = %q", got.Header.Get("Content-Type"))
		}
	} else if len(got.Body) != 0 {
		t.Errorf("unexpected body %s", got.Body)
	}
	if key := str(obj(w["headers"])["X-API-KEY"]); got.Header.Get("X-API-KEY") != key {
		t.Errorf("X-API-KEY = %q, want %q", got.Header.Get("X-API-KEY"), key)
	}
	if got.Header.Get("User-Agent") != "finlight-client-go/"+Version || got.Header.Get("x-client-version") != Version {
		t.Errorf("UA = %q, x-client-version = %q", got.Header.Get("User-Agent"), got.Header.Get("x-client-version"))
	}
}

// searchParamsFrom maps fixture args onto SearchParams through the wire keys,
// so that a fixture key without a field fails the test.
func searchParamsFrom(t *testing.T, args map[string]any) SearchParams {
	t.Helper()
	var p SearchParams
	rv := reflect.ValueOf(&p).Elem()
	for k, v := range args {
		f := rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, k) })
		if !f.IsValid() {
			t.Fatalf("SearchParams has no field for %q", k)
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(str(v))
		case reflect.Int:
			f.SetInt(int64(v.(float64)))
		case reflect.Slice:
			list := v.([]any)
			s := reflect.MakeSlice(f.Type(), len(list), len(list))
			for i, x := range list {
				s.Index(i).SetString(str(x))
			}
			f.Set(s)
		case reflect.Pointer:
			nv := reflect.New(f.Type().Elem())
			nv.Elem().Set(reflect.ValueOf(v).Convert(f.Type().Elem()))
			f.Set(nv)
		default:
			t.Fatalf("unhandled kind for %s", k)
		}
	}
	return p
}

func includes(v any) []IncludeBlock {
	var out []IncludeBlock
	for _, x := range v.([]any) {
		out = append(out, IncludeBlock(str(x)))
	}
	return out
}

var _ = conformance("R-01", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/search-minimal.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	page, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Search(t.Context(), searchParamsFrom(t, obj(obj(f["sdk"])["args"])))
	if err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	if string(r.Body) != `{"query":"ticker:XMPL"}` {
		t.Errorf("body = %s", r.Body)
	}
	body := obj(obj(f["response"])["body"])
	if len(page.Articles) != len(body["articles"].([]any)) || page.Pagination.Returned != 5 || page.Pagination.NextCursor != nil {
		t.Errorf("page = %+v", page.Pagination)
	}
})

var _ = conformance("R-02", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/search-all-parameters.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	args := obj(obj(f["sdk"])["args"])
	p := searchParamsFrom(t, args)
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	p.From = from.UTC().Format("2006-01-02T15:04:05.000Z")
	page, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Search(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	var body map[string]json.RawMessage
	_ = json.Unmarshal(r.Body, &body)
	if string(body["limit"]) != "50" || string(body["minRelevance"]) != "0.5" || string(body["order"]) != `"asc"` {
		t.Errorf("number/enum formatting: %s", r.Body)
	}
	if len(page.Articles) != 5 {
		t.Errorf("articles = %d", len(page.Articles))
	}
})

var _ = conformance("R-03", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/search-no-parameters.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	if _, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Search(t.Context(), SearchParams{}); err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	if string(r.Body) != "{}" {
		t.Errorf("body = %s, want {}", r.Body)
	}
})

var _ = conformance("R-04", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/get-by-id.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	args := obj(obj(f["sdk"])["args"])
	a, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Get(t.Context(), str(args["id"]), &GetOptions{Include: includes(args["include"])})
	if err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	q, _ := url.ParseQuery(r.RawQuery)
	if q.Get("include") != "content,events" || len(q["include"]) != 1 {
		t.Errorf("include = %v", q["include"])
	}
	if a.ID != args["id"] || !a.ContentPresent() || !a.EventsPresent() {
		t.Errorf("article = %+v", a)
	}
})

var _ = conformance("R-05", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/get-by-link.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	args := obj(obj(f["sdk"])["args"])
	a, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.GetByLink(t.Context(), str(args["link"]), &GetOptions{Include: includes(args["include"])})
	if err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	q, _ := url.ParseQuery(r.RawQuery)
	if !slices.Equal(q["link"], []string{str(args["link"])}) {
		t.Errorf("link = %v (encoded once?) raw %s", q["link"], r.RawQuery)
	}
	if a.Link != args["link"] {
		t.Errorf("link = %s", a.Link)
	}
})

var _ = conformance("R-06", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/companies-search.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	args := obj(obj(f["sdk"])["args"])
	list, err := newTestClient(t, Config{BaseURL: m.URL()}).Companies.Search(t.Context(), CompanySearchParams{Q: str(args["q"]), MIC: str(args["mic"])})
	if err != nil {
		t.Fatal(err)
	}
	expectRequest(t, m.reqs()[0], f["request"])
	if len(list.Items) != 1 || list.Items[0].CompanyID != 900001 || len(list.Skipped) != 0 {
		t.Errorf("companies = %+v", list)
	}
})

var _ = conformance("R-07", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/companies-get-many.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	list, err := newTestClient(t, Config{BaseURL: m.URL()}).Companies.GetMany(t.Context(), []int64{900001, 900002})
	if err != nil {
		t.Fatal(err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	if q, _ := url.ParseQuery(r.RawQuery); q.Get("ids") != "900001,900002" {
		t.Errorf("ids = %q", q.Get("ids"))
	}
	byID := map[int64]CompanyRecord{}
	for _, c := range list.Items {
		byID[c.CompanyID] = c
	}
	private := byID[900002]
	if private.PrimaryListing != nil || private.Listings == nil || len(private.Listings) != 0 || len(private.ISINs) != 0 {
		t.Errorf("private company = %+v", private)
	}
})

var _ = conformance("R-08", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/companies-get.json")
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	m.serveFixture(f["response"])
	c := newTestClient(t, Config{BaseURL: m.URL()})
	rec, err := c.Companies.Get(t.Context(), 900001, nil)
	if err != nil {
		t.Fatal(err)
	}
	expectRequest(t, m.reqs()[0], f["request"])
	hdr := obj(obj(f["response"])["headers"])
	ci := rec.CacheInfo()
	if ci.ETag != hdr["etag"] || ci.CacheControl != hdr["cache-control"] {
		t.Errorf("cache info = %+v", ci)
	}
	// The SDK keeps no company cache: a second Get without IfNoneMatch is a
	// plain request again, with no If-None-Match of its own.
	if _, err := c.Companies.Get(t.Context(), 900001, nil); err != nil {
		t.Fatal(err)
	}
	for i, r := range m.reqs() {
		if v, ok := r.Header["If-None-Match"]; ok {
			t.Errorf("request %d sent If-None-Match %q without a caller ETag", i, v)
		}
	}
	mics := map[string]bool{}
	for _, l := range rec.Listings {
		if mics[*l.MIC] {
			t.Errorf("mic %s twice", *l.MIC)
		}
		mics[*l.MIC] = true
	}
	if rec.Listings[0].Ticker != rec.PrimaryListing.Ticker || *rec.Listings[0].MIC != *rec.PrimaryListing.MIC {
		t.Error("primary listing must come first")
	}
	if rec.Listings[1].Active == nil || *rec.Listings[1].Active {
		t.Error("expired listing must read active=false")
	}
	if tm, ok := rec.UpdatedAtTime(); !ok || tm.Year() != 2026 {
		t.Error("updatedAt")
	}
})

var _ = conformance("R-09", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/companies-get-not-modified.json")
	args := obj(obj(f["sdk"])["args"])
	etag := str(args["ifNoneMatch"])

	// A caller-supplied ETag: 304 is "not modified", the language null.
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	c := newTestClient(t, Config{BaseURL: m.URL()})
	rec, err := c.Companies.Get(t.Context(), 900001, &CompanyGetOptions{IfNoneMatch: etag})
	if err != nil || rec != nil {
		t.Fatalf("Get = %+v, %v; want nil, nil", rec, err)
	}
	r := m.reqs()[0]
	expectRequest(t, r, f["request"])
	if r.Header.Get("If-None-Match") != etag {
		t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
	}

	// The SDK never answers a 304 with a stored record: even an unexpected 304
	// without a caller ETag is "not modified", never an error or a decode.
	m2 := newHTTPMock(t)
	m2.serveFixture(f["response"])
	rec, err = newTestClient(t, Config{BaseURL: m2.URL()}).Companies.Get(t.Context(), 900001, nil)
	if err != nil || rec != nil {
		t.Fatalf("Get without ETag on 304 = %+v, %v; want nil, nil", rec, err)
	}
})

func sourcesCase(t *testing.T, rel string) *SourceList {
	f := loadFixture(t, rel)
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	key := str(obj(obj(f["request"])["headers"])["X-API-KEY"])
	list, err := newTestClient(t, Config{BaseURL: m.URL(), APIKey: key}).Sources.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	expectRequest(t, m.reqs()[0], f["request"])
	if len(list.Items) != len(obj(obj(f["response"])["body"])["sources"].([]any)) {
		t.Fatalf("sources = %d", len(list.Items))
	}
	return list
}

var _ = conformance("R-10", func(t *testing.T) {
	list := sourcesCase(t, "fixtures/requests/sources-list-full.json")
	for _, s := range list.Items {
		if !s.HasContent.Present() {
			t.Errorf("%s: hasContent absent", s.Domain)
		}
		if s.Domain == "custom.example.net" && (!s.IsCustom || s.IsDefault || !s.HasContent.Value()) {
			t.Errorf("custom source = %+v", s)
		}
		if s.Domain == "archive.example.org" && (s.HasContent.Value() || s.HasContent.IsNull()) {
			t.Errorf("hasContent false must read false: %+v", s.HasContent)
		}
	}
	if list.Items[2].IsDefault != true {
		t.Error("isDefault lands on IsDefault")
	}
	// type is an open SourceType, nil when the source is unclassified.
	if list.Items[0].Type != nil || list.Items[1].Type == nil || *list.Items[1].Type != SourceTypeBlog ||
		list.Items[2].Type == nil || *list.Items[2].Type != SourceTypeFinancialMedia {
		t.Errorf("source types = %v, %v, %v", list.Items[0].Type, list.Items[1].Type, list.Items[2].Type)
	}
})

var _ = conformance("R-11", func(t *testing.T) {
	list := sourcesCase(t, "fixtures/requests/sources-list-basic.json")
	for _, s := range list.Items {
		if s.HasContent.Present() {
			t.Errorf("%s: hasContent must be absent (unknown), not false", s.Domain)
		}
		if s.IsCustom {
			t.Errorf("%s: not granted custom source listed", s.Domain)
		}
	}
	out, _ := json.Marshal(list.Items[0])
	if strings.Contains(string(out), "hasContent") {
		t.Errorf("absent hasContent written back: %s", out)
	}
})

// searchWireKeys is the set of request keys SearchParams can produce.
func searchWireKeys(t *testing.T) []string {
	t.Helper()
	yes, rel := true, 0.5
	p := SearchParams{
		Query: "q", Sources: []string{"a"}, OptInSources: []string{"a"}, ExcludeSources: []string{"a"},
		SourceTypes: []SourceType{"blog"}, ExcludeSourceTypes: []SourceType{"blog"},
		From: "f", To: "t", Languages: []string{"en"}, Tickers: []string{"X"}, Countries: []string{"US"},
		Categories: []string{"c"}, HasContent: &yes, Roles: []CompanyRole{"r"}, MinRelevance: &rel,
		EventTypes: []EventType{"e"}, EventDirections: []EventDirection{"d"}, MinImportance: "low",
		Sectors: []string{"s"}, CompanyCountries: []string{"US"}, Exchanges: []string{"X"},
		OrderBy: "publishedAt", Order: "asc", Limit: 5, Cursor: "c", Include: []IncludeBlock{"content"},
	}
	if n := reflect.TypeOf(p).NumField(); n != 26 {
		t.Fatalf("SearchParams has %d fields; update this test", n)
	}
	b, err := p.body()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var _ = conformance("R-12", func(t *testing.T) {
	f := loadFixture(t, "fixtures/requests/search-unknown-key.json")
	// The key set of SearchParams is closed: Go's type system rejects an
	// unknown field at compile time, so "eventType" cannot reach the wire.
	keys := searchWireKeys(t)
	for k := range obj(obj(f["sdk"])["args"]) {
		if slices.Contains(keys, k) {
			t.Errorf("%q is a sendable key", k)
		}
		if _, ok := reflect.TypeOf(SearchParams{}).FieldByNameFunc(func(n string) bool { return n == k || strings.EqualFold(n, k) }); ok {
			t.Errorf("SearchParams has a field for %q", k)
		}
	}
	// optInSources cannot be combined with sources, so R-02 leaves it out.
	want := []string{"cursor", "optInSources"}
	for k := range obj(obj(loadFixture(t, "fixtures/requests/search-all-parameters.json")["request"])["body"]) {
		want = append(want, k)
	}
	sort.Strings(want)
	if !slices.Equal(keys, want) {
		t.Errorf("sendable keys = %v, want %v", keys, want)
	}
})

// ---- P: pagination ----

var _ = conformance("P-01", func(t *testing.T) {
	f := loadFixture(t, "fixtures/pagination/three-pages.json")
	m := newHTTPMock(t)
	steps := f["steps"].([]any)
	for _, s := range steps {
		m.serveFixture(obj(s)["response"])
	}
	m.serveFixture(obj(steps[0])["response"]) // a fourth page must never be fetched
	args := obj(obj(f["sdk"])["args"])
	p := searchParamsFrom(t, args)
	res, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.SearchAll(t.Context(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Sources[0] = "changed.example.com" // the body was frozen at the call
	var ids []string
	for a, err := range res.All() {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.ID)
	}
	reqs := m.reqs()
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	var want []string
	for i, s := range steps {
		expectRequest(t, reqs[i], obj(s)["request"])
		for _, a := range obj(obj(obj(s)["response"])["body"])["articles"].([]any) {
			want = append(want, str(obj(a)["id"]))
		}
	}
	if !slices.Equal(ids, want) || len(ids) != 5 {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	// Bodies 2 and 3 equal body 1 apart from the cursor.
	strip := func(b []byte) string {
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		delete(m, "cursor")
		out, _ := json.Marshal(m)
		return string(out)
	}
	if strip(reqs[1].Body) != string(reqs[0].Body) || strip(reqs[2].Body) != string(reqs[0].Body) {
		t.Errorf("later bodies differ from page 1 beyond the cursor")
	}
	// One X-Request-Id per page.
	idsSeen := map[string]bool{}
	for _, r := range reqs {
		idsSeen[r.Header.Get("X-Request-Id")] = true
	}
	if len(idsSeen) != 3 {
		t.Errorf("X-Request-Id must differ between pages")
	}
	if res.PagesFetched() != 3 || res.Truncated() || res.NextCursor() != nil {
		t.Errorf("pages %d truncated %v next %v", res.PagesFetched(), res.Truncated(), res.NextCursor())
	}
	// A second iteration fails instead of searching again.
	for _, err := range res.All() {
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("second iteration: %v", err)
		}
	}
	if len(m.reqs()) != 3 {
		t.Error("second iteration sent a request")
	}
})

// replayPage sends a fixture request body through Search and returns the error.
func replayPage(t *testing.T, rel string) (*ArticlePage, *httpMock, map[string]any, error) {
	t.Helper()
	f := loadFixture(t, rel)
	m := newHTTPMock(t)
	m.serveFixture(f["response"])
	body := obj(obj(f["request"])["body"])
	page, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.Search(t.Context(), searchParamsFrom(t, body))
	expectRequest(t, m.reqs()[0], f["request"])
	return page, m, f, err
}

var _ = conformance("P-02", func(t *testing.T) {
	page, _, _, err := replayPage(t, "fixtures/pagination/resize-and-reproject.json")
	if err != nil || page.Pagination.Limit != 3 || page.Pagination.HasMore {
		t.Fatalf("page = %+v, %v", page, err)
	}
	for _, a := range page.Articles {
		if !a.ContentPresent() {
			t.Errorf("include changed on page 2 must deliver content")
		}
	}
})

func cursorMismatch(t *testing.T, rel string) {
	t.Helper()
	_, m, _, err := replayPage(t, rel)
	var ae *APIError
	if !errors.As(err, &ae) || ae.APIKind != APIKindCursorMismatch || ae.Code != ProblemCodeCursorMismatch || len(m.reqs()) != 1 {
		t.Fatalf("err = %v", err)
	}
}

// pagerBodies runs SearchAll over two pages and returns the request bodies.
func pagerBodies(t *testing.T) []map[string]any {
	t.Helper()
	f := loadFixture(t, "fixtures/pagination/three-pages.json")
	m := newHTTPMock(t)
	for _, s := range f["steps"].([]any) {
		m.serveFixture(obj(s)["response"])
	}
	res, err := newTestClient(t, Config{BaseURL: m.URL()}).Articles.SearchAll(t.Context(), SearchParams{Sources: []string{"news.example.com"}, Limit: 2}, &SearchAllOptions{MaxPages: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range res.Pages() {
		if err != nil {
			t.Fatal(err)
		}
	}
	var out []map[string]any
	for _, r := range m.reqs() {
		var b map[string]any
		_ = json.Unmarshal(r.Body, &b)
		out = append(out, b)
	}
	if !res.Truncated() || res.NextCursor() == nil || res.PagesFetched() != 2 {
		t.Errorf("maxPages stop: truncated %v next %v", res.Truncated(), res.NextCursor())
	}
	return out
}

var _ = conformance("P-03", func(t *testing.T) {
	cursorMismatch(t, "fixtures/pagination/injected-to.json")
	for _, b := range pagerBodies(t) {
		if _, ok := b["to"]; ok {
			t.Error("the pager injected to")
		}
	}
})

var _ = conformance("P-04", func(t *testing.T) {
	cursorMismatch(t, "fixtures/pagination/injected-default-languages.json")
	for _, b := range pagerBodies(t) {
		if _, ok := b["languages"]; ok {
			t.Error("the pager injected languages")
		}
	}
})

var _ = conformance("P-05", func(t *testing.T) {
	cursorMismatch(t, "fixtures/pagination/changed-order.json")
	_, _, f, err := replayPage(t, "fixtures/pagination/changed-order.json")
	var ae *APIError
	errors.As(err, &ae)
	if !strings.Contains(ae.Detail, "publishedAt:desc") || !strings.Contains(ae.Detail, "publishedAt:asc") || ae.Errors[0].Field != "cursor" {
		t.Errorf("detail = %q", ae.Detail)
	}
	_ = f
})

var _ = conformance("P-06", func(t *testing.T) {
	page, _, f, err := replayPage(t, "fixtures/pagination/reordered-list.json")
	if err != nil || !page.Pagination.HasMore {
		t.Fatalf("page = %+v, %v", page, err)
	}
	next := str(obj(obj(obj(f["response"])["body"])["pagination"])["nextCursor"])
	if page.Pagination.NextCursor == nil || *page.Pagination.NextCursor != next {
		t.Errorf("nextCursor = %v", page.Pagination.NextCursor)
	}
})

// ---- E: errors and retry classes ----

func errorCase(id, file string) bool {
	return conformance(id, func(t *testing.T) { runErrorCase(t, file) })
}

var (
	_ = errorCase("E-01", "validation-enum.json")
	_ = errorCase("E-02", "validation-multiple.json")
	_ = errorCase("E-03", "validation-query-language.json")
	_ = errorCase("E-04", "validation-empty-languages.json")
	_ = errorCase("E-05", "validation-include-query.json")
	_ = errorCase("E-06", "companies-neither.json")
	_ = errorCase("E-07", "companies-both.json")
	_ = errorCase("E-08", "companies-ids-not-integer.json")
	_ = errorCase("E-09", "company-id-not-integer.json")
	_ = errorCase("E-10", "cursor-invalid.json")
	_ = errorCase("E-11", "cursor-version.json")
	_ = errorCase("E-12", "unauthorized-missing-key.json")
	_ = errorCase("E-13", "unauthorized-invalid-key.json")
	_ = errorCase("E-14", "forbidden-companies.json")
	_ = errorCase("E-15", "not-found-id.json")
	_ = errorCase("E-16", "not-found-link.json")
	_ = errorCase("E-17", "not-found-company.json")
	_ = errorCase("E-18", "rate-limited-monthly.json")
	_ = errorCase("E-19", "rate-limited-burst.json")
	_ = errorCase("E-20", "internal-error.json")
	_ = errorCase("E-21", "malformed-json-body.json")
	_ = errorCase("E-22", "unknown-route.json")
	_ = errorCase("E-23", "generated-request-id.json")
	_ = errorCase("E-24", "gateway-non-json.json")
	_ = errorCase("E-25", "cursor-mismatch.json")
)

// TestErrorFixturesAreCases checks that every error fixture is a numbered case.
func TestErrorFixturesAreCases(t *testing.T) {
	entries, _ := os.ReadDir(filepath.Join(kitDir, "fixtures/errors"))
	n := 0
	for _, e := range entries {
		if e.Name() != "retry-classes.json" {
			n++
		}
	}
	if n != 25 {
		t.Fatalf("error fixtures = %d, want 25", n)
	}
}

func runErrorCase(t *testing.T, file string) {
	f := loadFixture(t, "fixtures/errors/"+file)
	req := obj(f["request"])
	resp := obj(f["response"])
	expect := obj(f["expect"])
	hdr := obj(req["headers"])

	m := newHTTPMock(t)
	m.setAlways(fixtureResponse(resp))
	clock := &instantClock{}
	key := str(hdr["X-API-KEY"])
	if key == "" {
		key = "sk_missing"
	}
	c := newTestClient(t, Config{BaseURL: m.URL(), APIKey: key, Clock: clock})

	ar := apiRequest{method: str(req["method"]), path: str(req["path"]), requestID: str(hdr["X-Request-Id"])}
	if q := str(req["query"]); q != "" {
		ar.query, _ = url.ParseQuery(q)
	}
	if raw, ok := req["rawBody"]; ok {
		ar.body = []byte(str(raw))
	} else if b, ok := req["body"]; ok {
		ar.body = mustJSON(t, b)
	}
	_, err := c.t.do(t.Context(), ar)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	var fe Error
	if !errors.As(err, &fe) || fe.Kind() != ErrorKindAPI {
		t.Errorf("kind = %v", fe)
	}
	if ae.Status != int(resp["status"].(float64)) {
		t.Errorf("status = %d", ae.Status)
	}
	body, isObj := resp["body"].(map[string]any)
	switch code := str(expect["code"]); code {
	case "NESTJS_FALLBACK":
		if ae.Code != "" || ae.Detail != body["message"] {
			t.Errorf("fallback: code %q detail %q", ae.Code, ae.Detail)
		}
	case "":
		if ae.Code != "" {
			t.Errorf("code = %q, want none", ae.Code)
		}
	default:
		if string(ae.Code) != code {
			t.Errorf("code = %q, want %q", ae.Code, code)
		}
	}
	if string(ae.APIKind) != str(expect["apiKind"]) {
		t.Errorf("apiKind = %s, want %s", ae.APIKind, expect["apiKind"])
	}
	if ae.Raw != bodyText(resp["body"]) {
		t.Errorf("raw = %q", ae.Raw)
	}
	if isObj && !jsonEqual(t, ae.RawJSON(), mustJSON(t, body)) {
		t.Errorf("RawJSON differs")
	}
	reqs := m.reqs()
	sentID := reqs[0].Header.Get("X-Request-Id")
	wantID := sentID
	if isObj && body["requestId"] != nil {
		wantID = str(body["requestId"])
	}
	if ae.RequestID != wantID {
		t.Errorf("requestId = %q, want %q", ae.RequestID, wantID)
	}
	if id := str(hdr["X-Request-Id"]); id != "" && sentID != id {
		t.Errorf("sent X-Request-Id %q, want %q", sentID, id)
	}
	if isObj {
		if list, ok := body["errors"].([]any); ok {
			if len(ae.Errors) != len(list) {
				t.Fatalf("errors = %+v", ae.Errors)
			}
			for i, e := range list {
				fe := obj(e)
				if ae.Errors[i].Field != fe["field"] || ae.Errors[i].Issue != fe["issue"] {
					t.Errorf("errors[%d] = %+v", i, ae.Errors[i])
				}
				if av, ok := fe["allowedValues"]; ok && !slices.Equal(ae.Errors[i].AllowedValues, anyStrings(av)) {
					t.Errorf("allowedValues[%d] = %v", i, ae.Errors[i].AllowedValues)
				}
			}
		}
	}
	waits := clock.sleeps()
	switch str(expect["retry"]) {
	case "never":
		if len(reqs) != 1 || len(waits) != 0 {
			t.Errorf("attempts = %d, waits = %v; want no retry", len(reqs), waits)
		}
	case "backoff":
		if len(reqs) != 4 || len(waits) != 3 {
			t.Errorf("attempts = %d, waits = %v; want 1 + 3 retries", len(reqs), waits)
		}
	case "once-after-burst-wait":
		if len(reqs) != 2 || len(waits) != 1 || waits[0] != 7*time.Second || ae.RetryAfter != 7*time.Second {
			t.Errorf("attempts = %d, waits = %v, retryAfter = %v", len(reqs), waits, ae.RetryAfter)
		}
	default:
		t.Fatalf("unknown retry class %v", expect["retry"])
	}
	for _, r := range reqs {
		if r.Header.Get("X-Request-Id") != sentID {
			t.Error("X-Request-Id changed between retries")
		}
	}
	switch f["id"] {
	case "E-02":
		if !regexpMatch(`^\d+ request fields are not valid\.$`, ae.Detail) {
			t.Errorf("detail = %q", ae.Detail)
		}
	case "E-06", "E-07":
		// The SDK rejects these shapes locally before any request.
		m2 := newHTTPMock(t)
		c2 := newTestClient(t, Config{BaseURL: m2.URL()})
		_, err := c2.Companies.Search(t.Context(), CompanySearchParams{})
		var ve *ValidationError
		if !errors.As(err, &ve) || len(m2.reqs()) != 0 {
			t.Errorf("local check: %v", err)
		}
	case "E-08":
		m2 := newHTTPMock(t)
		_, err := newTestClient(t, Config{BaseURL: m2.URL()}).Companies.GetMany(t.Context(), []int64{900001, 0})
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Param != "ids" || len(m2.reqs()) != 0 {
			t.Errorf("local check: %v", err)
		}
	case "E-04":
		m2 := newHTTPMock(t)
		_, err := newTestClient(t, Config{BaseURL: m2.URL()}).Articles.Search(t.Context(), SearchParams{Languages: []string{}})
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Param != "languages" || len(m2.reqs()) != 0 {
			t.Errorf("local check: %v", err)
		}
	case "E-18":
		if ae.LimitType != RateLimitTypeMonthly || ae.RetryAfter != 0 {
			t.Errorf("limitType %s retryAfter %v", ae.LimitType, ae.RetryAfter)
		}
		// searchAll stops with the error at once, never waits.
		m2 := newHTTPMock(t)
		m2.setAlways(fixtureResponse(resp))
		clock2 := &instantClock{}
		res, _ := newTestClient(t, Config{BaseURL: m2.URL(), Clock: clock2}).Articles.SearchAll(t.Context(), SearchParams{}, nil)
		n := 0
		for _, err := range res.All() {
			n++
			if !errors.As(err, &ae) || ae.LimitType != RateLimitTypeMonthly {
				t.Errorf("searchAll err = %v", err)
			}
		}
		if n != 1 || len(m2.reqs()) != 1 || len(clock2.sleeps()) != 0 {
			t.Errorf("searchAll retried or waited: %d requests, waits %v", len(m2.reqs()), clock2.sleeps())
		}
	case "E-19":
		if ae.LimitType != RateLimitTypeBurst {
			t.Errorf("limitType = %s", ae.LimitType)
		}
		// Off: thrown at once.
		m2 := newHTTPMock(t)
		m2.setAlways(fixtureResponse(resp))
		clock2 := &instantClock{}
		_, err := newTestClient(t, Config{BaseURL: m2.URL(), Clock: clock2, RetryOnBurst: BurstRetryOff}).t.do(t.Context(), apiRequest{method: "POST", path: "/v3/articles/search", body: []byte("{}")})
		if !errors.As(err, &ae) || len(m2.reqs()) != 1 || len(clock2.sleeps()) != 0 {
			t.Errorf("burst retry off: %d requests, waits %v", len(m2.reqs()), clock2.sleeps())
		}
	case "E-20":
		if ae.Detail != "The request could not be processed." || reqs[0].Method != "POST" {
			t.Errorf("detail = %q", ae.Detail)
		}
	case "E-23":
		if !strings.HasPrefix(ae.RequestID, "req_") {
			t.Errorf("requestId = %q", ae.RequestID)
		}
	}
}
