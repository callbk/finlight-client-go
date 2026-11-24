package finlight

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// SearchParams are the parameters of Articles.Search and Articles.SearchAll.
// The SDK sends only what is set: an empty string, a nil slice and a nil
// pointer mean "not set", and no default is ever added. Pointers mark the
// parameters where false or 0 is a real value. Vocabulary values are not
// checked locally; the server answers unknown values with a 400.
type SearchParams struct {
	Query          string
	Sources        []string
	OptInSources   []string
	ExcludeSources []string
	// SourceTypes keeps articles whose source has one of these types;
	// ExcludeSourceTypes drops them. Both filter inside the source pool and
	// never add a source to it. Articles of unclassified sources (source.type
	// null) never match SourceTypes and are kept by ExcludeSourceTypes.
	SourceTypes        []SourceType
	ExcludeSourceTypes []SourceType
	// From and To are sent unchanged. Send UTC with milliseconds, e.g.
	// t.UTC().Format("2006-01-02T15:04:05.000Z"), or a date "2006-01-02".
	From             string
	To               string
	Languages        []string // must not be empty when set
	Tickers          []string
	Countries        []string
	Categories       []string
	HasContent       *bool
	Roles            []CompanyRole
	MinRelevance     *float64 // 0 to 1
	EventTypes       []EventType
	EventDirections  []EventDirection
	MinImportance    Importance
	Sectors          []string
	CompanyCountries []string
	Exchanges        []string
	OrderBy          OrderBy
	Order            SortOrder
	Limit            int // 1 to 100; 0 means not set (server default 20)
	Cursor           string
	Include          []IncludeBlock // at most 3 values
}

// body builds the request body from the fixed key list. Slices are copied, so
// later changes by the caller do not reach the frozen body of SearchAll.
func (p SearchParams) body() (map[string]any, error) {
	if p.Limit < 0 || p.Limit > 100 {
		return nil, validationErr("limit", "limit must be an integer from 1 to 100, got %d", p.Limit)
	}
	if p.Languages != nil && len(p.Languages) == 0 {
		return nil, validationErr("languages", "languages must not be empty")
	}
	// Written as !(0 <= v <= 1) so that NaN fails too.
	if p.MinRelevance != nil && !(*p.MinRelevance >= 0 && *p.MinRelevance <= 1) {
		return nil, validationErr("minRelevance", "minRelevance must be between 0 and 1")
	}
	if err := checkInclude(p.Include); err != nil {
		return nil, err
	}
	b := map[string]any{}
	setStr := func(k, v string) {
		if v != "" {
			b[k] = v
		}
	}
	setList := func(k string, v []string) {
		if v != nil {
			b[k] = append([]string{}, v...)
		}
	}
	setStr("query", p.Query)
	setList("sources", p.Sources)
	setList("optInSources", p.OptInSources)
	setList("excludeSources", p.ExcludeSources)
	setList("sourceTypes", toStrings(p.SourceTypes))
	setList("excludeSourceTypes", toStrings(p.ExcludeSourceTypes))
	setStr("from", p.From)
	setStr("to", p.To)
	setList("languages", p.Languages)
	setList("tickers", p.Tickers)
	setList("countries", p.Countries)
	setList("categories", p.Categories)
	if p.HasContent != nil {
		b["hasContent"] = *p.HasContent
	}
	setList("roles", toStrings(p.Roles))
	if p.MinRelevance != nil {
		b["minRelevance"] = *p.MinRelevance
	}
	setList("eventTypes", toStrings(p.EventTypes))
	setList("eventDirections", toStrings(p.EventDirections))
	setStr("minImportance", string(p.MinImportance))
	setList("sectors", p.Sectors)
	setList("companyCountries", p.CompanyCountries)
	setList("exchanges", p.Exchanges)
	setStr("orderBy", string(p.OrderBy))
	setStr("order", string(p.Order))
	if p.Limit != 0 {
		b["limit"] = p.Limit
	}
	setStr("cursor", p.Cursor)
	if len(p.Include) > 0 {
		b["include"] = toStrings(p.Include)
	}
	return b, nil
}

func toStrings[T ~string](v []T) []string {
	if v == nil {
		return nil
	}
	out := make([]string, len(v))
	for i, s := range v {
		out[i] = string(s)
	}
	return out
}

func checkInclude(include []IncludeBlock) error {
	if len(include) > 3 {
		return validationErr("include", "include takes at most 3 values, got %d", len(include))
	}
	return nil
}

// marshalBody writes a request body; an empty body is {} (never null or []).
// body() rejects every value JSON cannot hold, so an error here is a bug; it
// still comes back as a *ValidationError instead of a panic.
func marshalBody(b map[string]any) ([]byte, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, &ValidationError{Message: "cannot encode the request body: " + err.Error()}
	}
	return data, nil
}

// GetOptions are the options of Articles.Get and Articles.GetByLink.
type GetOptions struct {
	Include []IncludeBlock // at most 3 values
}

// SearchAllOptions are the options of Articles.SearchAll.
type SearchAllOptions struct {
	MaxPages int // 0 uses Config.MaxPages (default 10)
}

// CompanySearchParams are the parameters of Companies.Search.
type CompanySearchParams struct {
	Q   string // required, at most 200 characters
	MIC string // optional, at most 32 characters; narrows a Q lookup
}

func (p CompanySearchParams) validate() error {
	if strings.TrimSpace(p.Q) == "" {
		return validationErr("q", "q is required")
	}
	if utf8.RuneCountInString(p.Q) > 200 {
		return validationErr("q", "q has at most 200 characters")
	}
	if utf8.RuneCountInString(p.MIC) > 32 {
		return validationErr("mic", "mic has at most 32 characters")
	}
	return nil
}

// CompanyGetOptions are the options of Companies.Get.
type CompanyGetOptions struct {
	// IfNoneMatch is an ETag from an earlier response. When the server answers
	// 304, Get returns (nil, nil): not modified.
	IfNoneMatch string
}

// StreamParams are the parameters of an enriched stream. The subscribe frame
// carries only what is set, plus payloadVersion 3 and a clientNonce.
type StreamParams struct {
	Query          string
	Sources        []string
	ExcludeSources []string
	OptInSources   []string
	// SourceTypes and ExcludeSourceTypes filter by the type of the article's
	// source, as in SearchParams. Unknown values are not checked locally: the
	// server ends the stream with a terminal VALIDATION_FAILED.
	SourceTypes         []SourceType
	ExcludeSourceTypes  []SourceType
	Language            string // one language; the server default is "en"
	Tickers             []string
	Countries           []string
	Categories          []string
	ExcludeEmptyContent *bool
	IncludeUpdates      *bool // without it no article.updated is delivered
	Include             []IncludeBlock
}

// RawStreamParams are the parameters of a raw stream. Raw streams deliver the
// article skeleton only and know no include.
type RawStreamParams struct {
	Query          string
	Sources        []string
	ExcludeSources []string
	// SourceTypes and ExcludeSourceTypes work as in StreamParams.
	SourceTypes        []SourceType
	ExcludeSourceTypes []SourceType
	Language           string
	IncludeUpdates     *bool
}

func (p StreamParams) subscribe() (map[string]any, error) {
	if err := checkInclude(p.Include); err != nil {
		return nil, err
	}
	m := map[string]any{}
	setStr(m, "query", p.Query)
	setList(m, "sources", p.Sources)
	setList(m, "excludeSources", p.ExcludeSources)
	setList(m, "optInSources", p.OptInSources)
	setList(m, "sourceTypes", toStrings(p.SourceTypes))
	setList(m, "excludeSourceTypes", toStrings(p.ExcludeSourceTypes))
	setStr(m, "language", p.Language)
	setList(m, "tickers", p.Tickers)
	setList(m, "countries", p.Countries)
	setList(m, "categories", p.Categories)
	setBool(m, "excludeEmptyContent", p.ExcludeEmptyContent)
	setBool(m, "includeUpdates", p.IncludeUpdates)
	if len(p.Include) > 0 {
		m["include"] = toStrings(p.Include)
	}
	return m, nil
}

func (p RawStreamParams) subscribe() (map[string]any, error) {
	m := map[string]any{}
	setStr(m, "query", p.Query)
	setList(m, "sources", p.Sources)
	setList(m, "excludeSources", p.ExcludeSources)
	setList(m, "sourceTypes", toStrings(p.SourceTypes))
	setList(m, "excludeSourceTypes", toStrings(p.ExcludeSourceTypes))
	setStr(m, "language", p.Language)
	setBool(m, "includeUpdates", p.IncludeUpdates)
	return m, nil
}

func setStr(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func setList(m map[string]any, k string, v []string) {
	if v != nil {
		m[k] = append([]string{}, v...)
	}
}

func setBool(m map[string]any, k string, v *bool) {
	if v != nil {
		m[k] = *v
	}
}
