package finlight

import (
	"encoding/json"
	"fmt"
	"time"
)

// Article is the v3 article object, identical on REST, WebSocket and webhooks.
//
// Keys outside the gated blocks are always present; a missing optional key
// reads as nil (or an empty list for Media, Countries and Categories). The
// gated keys (Content, Sentiment, Companies, Events) are Fields: absent when
// not entitled, not requested or raw; null when entitled but without value.
type Article struct {
	ID          string        `json:"id"`
	Link        string        `json:"link"`
	Source      ArticleSource `json:"source"`
	Title       string        `json:"title"`
	Summary     *string       `json:"summary"`
	Language    *string       `json:"language"`
	Media       []Media       `json:"media"`
	PublishedAt *string       `json:"publishedAt"` // as sent; see PublishedAtTime
	FirstSeenAt *string       `json:"firstSeenAt"`
	RevisedAt   *string       `json:"revisedAt"`
	AvailableAt *string       `json:"availableAt"` // always nil on raw
	Countries   []string      `json:"countries"`
	Categories  []string      `json:"categories"` // open vocabulary

	Content   Field[string]     `json:"content,omitzero"`
	Sentiment Field[Sentiment]  `json:"sentiment,omitzero"`
	Companies Field[[]Company]  `json:"companies,omitzero"`
	Events    Field[[]EventTag] `json:"events,omitzero"`

	raw json.RawMessage
}

// ArticleSource is the source block of an article.
type ArticleSource struct {
	Domain string `json:"domain"`
	// Type is the source type when the article was indexed; nil when the
	// source is unclassified or the article predates source types. It can lag
	// the type in /v3/sources after a reclassification.
	Type *SourceType `json:"type"`
}

// Media is one image of an article.
type Media struct {
	URL string `json:"url"`
}

// Sentiment is the sentiment block of an article.
type Sentiment struct {
	Label      *SentimentLabel `json:"label"`
	Confidence *float64        `json:"confidence"`
}

// Company is a company tagged in an article.
type Company struct {
	CompanyID      int64        `json:"companyId"`
	Name           string       `json:"name"`
	ISIN           *string      `json:"isin"`
	PrimaryListing *Listing     `json:"primaryListing"` // nil: no listing (e.g. private company)
	Confidence     *float64     `json:"confidence"`
	Role           *CompanyRole `json:"role"`
	Relevance      *float64     `json:"relevance"`
}

// Listing is a listing of a company. Active is only set inside
// CompanyRecord.Listings.
type Listing struct {
	Ticker  string  `json:"ticker"`
	MIC     *string `json:"mic"`
	Country *string `json:"country"`
	Active  *bool   `json:"active,omitempty"`
}

// EventTag is an event tagged in an article. CompanyIDs is present only when
// the payload also carries Companies.
type EventTag struct {
	Type       EventType       `json:"type"`
	Direction  *EventDirection `json:"direction"`
	Importance *Importance     `json:"importance"`
	Confidence *float64        `json:"confidence"`
	CompanyIDs Field[[]int64]  `json:"companyIds,omitzero"`
}

// ContentPresent reports whether the content key was in the JSON (null counts).
func (a *Article) ContentPresent() bool { return a.Content.Present() }

// SentimentPresent reports whether the sentiment key was in the JSON (null counts).
func (a *Article) SentimentPresent() bool { return a.Sentiment.Present() }

// CompaniesPresent reports whether the companies key was in the JSON.
func (a *Article) CompaniesPresent() bool { return a.Companies.Present() }

// EventsPresent reports whether the events key was in the JSON.
func (a *Article) EventsPresent() bool { return a.Events.Present() }

// RawJSON returns the article exactly as received.
func (a *Article) RawJSON() json.RawMessage { return a.raw }

// PublishedAtTime parses PublishedAt strictly as RFC 3339.
func (a *Article) PublishedAtTime() (time.Time, bool) { return parseTime(a.PublishedAt) }

// FirstSeenAtTime parses FirstSeenAt strictly as RFC 3339.
func (a *Article) FirstSeenAtTime() (time.Time, bool) { return parseTime(a.FirstSeenAt) }

// RevisedAtTime parses RevisedAt strictly as RFC 3339.
func (a *Article) RevisedAtTime() (time.Time, bool) { return parseTime(a.RevisedAt) }

// AvailableAtTime parses AvailableAt strictly as RFC 3339.
func (a *Article) AvailableAtTime() (time.Time, bool) { return parseTime(a.AvailableAt) }

// UnmarshalJSON decodes an article with exact key matching. It fails only when
// id, link, title or source.domain is missing, null or not a string, or when a
// present value has the wrong JSON type.
func (a *Article) UnmarshalJSON(b []byte) error {
	v, err := decodeArticle(b, "")
	if err != nil {
		return err
	}
	*a = *v
	return nil
}

// MarshalJSON writes the wire form: absent gated keys stay absent, null stays null.
func (a Article) MarshalJSON() ([]byte, error) {
	type wire Article
	w := wire(a)
	if w.Media == nil {
		w.Media = []Media{}
	}
	if w.Countries == nil {
		w.Countries = []string{}
	}
	if w.Categories == nil {
		w.Categories = []string{}
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes with exact key matching.
func (s *Sentiment) UnmarshalJSON(b []byte) error {
	v, err := decodeSentiment(b, "")
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// UnmarshalJSON decodes with exact key matching.
func (c *Company) UnmarshalJSON(b []byte) error {
	v, err := decodeCompany(b, "")
	if err != nil {
		return err
	}
	*c = v
	return nil
}

// UnmarshalJSON decodes with exact key matching.
func (e *EventTag) UnmarshalJSON(b []byte) error {
	v, err := decodeEvent(b, "")
	if err != nil {
		return err
	}
	*e = v
	return nil
}

func decodeArticle(raw json.RawMessage, path string) (*Article, error) {
	o, err := parseObject(raw, path)
	if err != nil {
		return nil, err
	}
	a := &Article{raw: append(json.RawMessage(nil), raw...)}
	if a.ID, err = o.reqString(path, "id"); err != nil {
		return nil, err
	}
	if a.Link, err = o.reqString(path, "link"); err != nil {
		return nil, err
	}
	if a.Title, err = o.reqString(path, "title"); err != nil {
		return nil, err
	}
	srcRaw, ok := o["source"]
	if !ok {
		return nil, decodeErr(sub(path, "source"), nil, "required key is missing")
	}
	src, err := parseObject(srcRaw, sub(path, "source"))
	if err != nil {
		return nil, err
	}
	if a.Source.Domain, err = src.reqString(sub(path, "source"), "domain"); err != nil {
		return nil, err
	}
	st, err := src.optString(sub(path, "source"), "type")
	if err != nil {
		return nil, err
	}
	if st != nil {
		t := SourceType(*st)
		a.Source.Type = &t
	}
	for _, f := range []struct {
		key string
		dst **string
	}{
		{"summary", &a.Summary}, {"language", &a.Language}, {"publishedAt", &a.PublishedAt},
		{"firstSeenAt", &a.FirstSeenAt}, {"revisedAt", &a.RevisedAt}, {"availableAt", &a.AvailableAt},
	} {
		if *f.dst, err = o.optString(path, f.key); err != nil {
			return nil, err
		}
	}
	media, _, err := o.array(path, "media")
	if err != nil {
		return nil, err
	}
	a.Media = make([]Media, 0, len(media))
	for i, m := range media {
		mp := fmt.Sprintf("%s[%d]", sub(path, "media"), i)
		mo, err := parseObject(m, mp)
		if err != nil {
			return nil, err
		}
		u, err := mo.plainString(mp, "url")
		if err != nil {
			return nil, err
		}
		a.Media = append(a.Media, Media{URL: u})
	}
	if a.Countries, err = o.stringList(path, "countries"); err != nil {
		return nil, err
	}
	if a.Categories, err = o.stringList(path, "categories"); err != nil {
		return nil, err
	}
	if c, ok := o["content"]; ok {
		if isJSONNull(c) {
			a.Content = fieldNullOf[string]()
		} else {
			s, err := o.reqString(path, "content")
			if err != nil {
				return nil, err
			}
			a.Content = fieldOf(s)
		}
	}
	if s, ok := o["sentiment"]; ok {
		if isJSONNull(s) {
			a.Sentiment = fieldNullOf[Sentiment]()
		} else {
			v, err := decodeSentiment(s, sub(path, "sentiment"))
			if err != nil {
				return nil, err
			}
			a.Sentiment = fieldOf(v)
		}
	}
	if elems, present, err := o.array(path, "companies"); err != nil {
		return nil, err
	} else if present {
		if elems == nil {
			a.Companies = fieldNullOf[[]Company]()
		} else {
			list := make([]Company, 0, len(elems))
			for i, e := range elems {
				c, err := decodeCompany(e, fmt.Sprintf("%s[%d]", sub(path, "companies"), i))
				if err != nil {
					return nil, err
				}
				list = append(list, c)
			}
			a.Companies = fieldOf(list)
		}
	}
	if elems, present, err := o.array(path, "events"); err != nil {
		return nil, err
	} else if present {
		if elems == nil {
			a.Events = fieldNullOf[[]EventTag]()
		} else {
			list := make([]EventTag, 0, len(elems))
			for i, e := range elems {
				ev, err := decodeEvent(e, fmt.Sprintf("%s[%d]", sub(path, "events"), i))
				if err != nil {
					return nil, err
				}
				list = append(list, ev)
			}
			a.Events = fieldOf(list)
		}
	}
	return a, nil
}

func decodeSentiment(raw json.RawMessage, path string) (Sentiment, error) {
	var s Sentiment
	o, err := parseObject(raw, path)
	if err != nil {
		return s, err
	}
	label, err := o.optString(path, "label")
	if err != nil {
		return s, err
	}
	if label != nil {
		l := SentimentLabel(*label)
		s.Label = &l
	}
	if s.Confidence, err = o.optFloat(path, "confidence"); err != nil {
		return s, err
	}
	return s, nil
}

func decodeListing(raw json.RawMessage, path string) (*Listing, error) {
	if isJSONNull(raw) {
		return nil, nil
	}
	o, err := parseObject(raw, path)
	if err != nil {
		return nil, err
	}
	l := &Listing{}
	if l.Ticker, err = o.plainString(path, "ticker"); err != nil {
		return nil, err
	}
	if l.MIC, err = o.optString(path, "mic"); err != nil {
		return nil, err
	}
	if l.Country, err = o.optString(path, "country"); err != nil {
		return nil, err
	}
	if l.Active, err = o.optBool(path, "active"); err != nil {
		return nil, err
	}
	return l, nil
}

func decodeCompany(raw json.RawMessage, path string) (Company, error) {
	var c Company
	o, err := parseObject(raw, path)
	if err != nil {
		return c, err
	}
	if c.CompanyID, err = o.reqInt64(path, "companyId"); err != nil {
		return c, err
	}
	if c.Name, err = o.reqString(path, "name"); err != nil {
		return c, err
	}
	if c.ISIN, err = o.optString(path, "isin"); err != nil {
		return c, err
	}
	if pl, ok := o["primaryListing"]; ok {
		if c.PrimaryListing, err = decodeListing(pl, sub(path, "primaryListing")); err != nil {
			return c, err
		}
	}
	if c.Confidence, err = o.optFloat(path, "confidence"); err != nil {
		return c, err
	}
	role, err := o.optString(path, "role")
	if err != nil {
		return c, err
	}
	if role != nil {
		r := CompanyRole(*role)
		c.Role = &r
	}
	if c.Relevance, err = o.optFloat(path, "relevance"); err != nil {
		return c, err
	}
	return c, nil
}

func decodeEvent(raw json.RawMessage, path string) (EventTag, error) {
	var e EventTag
	o, err := parseObject(raw, path)
	if err != nil {
		return e, err
	}
	t, err := o.reqString(path, "type")
	if err != nil {
		return e, err
	}
	e.Type = EventType(t)
	dir, err := o.optString(path, "direction")
	if err != nil {
		return e, err
	}
	if dir != nil {
		d := EventDirection(*dir)
		e.Direction = &d
	}
	imp, err := o.optString(path, "importance")
	if err != nil {
		return e, err
	}
	if imp != nil {
		i := Importance(*imp)
		e.Importance = &i
	}
	if e.Confidence, err = o.optFloat(path, "confidence"); err != nil {
		return e, err
	}
	elems, present, err := o.array(path, "companyIds")
	if err != nil {
		return e, err
	}
	if present {
		if elems == nil {
			e.CompanyIDs = fieldNullOf[[]int64]()
		} else {
			ids := make([]int64, 0, len(elems))
			for i, el := range elems {
				n, err := parseInt64(el, fmt.Sprintf("%s[%d]", sub(path, "companyIds"), i))
				if err != nil {
					return e, err
				}
				ids = append(ids, n)
			}
			e.CompanyIDs = fieldOf(ids)
		}
	}
	return e, nil
}

// CompanyRecord is a company from the company master data (/v3/companies).
type CompanyRecord struct {
	CompanyID      int64     `json:"companyId"`
	Name           string    `json:"name"`
	Country        *string   `json:"country"`
	Sector         *string   `json:"sector"`
	Industry       *string   `json:"industry"`
	ISINs          []string  `json:"isins"`
	PrimaryListing *Listing  `json:"primaryListing"`
	Listings       []Listing `json:"listings"` // one per exchange, the primary first
	UpdatedAt      *string   `json:"updatedAt"`

	raw   json.RawMessage
	cache CacheInfo
}

// CacheInfo carries the caching headers of a companies.get response. The SDK
// keeps no cache of its own; use them to cache the record yourself and pass
// ETag back as CompanyGetOptions.IfNoneMatch.
type CacheInfo struct {
	ETag         string // the ETag header ("" when none)
	CacheControl string // the Cache-Control header
}

// RawJSON returns the record exactly as received.
func (r *CompanyRecord) RawJSON() json.RawMessage { return r.raw }

// CacheInfo returns the ETag and Cache-Control of a companies.get response
// (zero for records from search or getMany).
func (r *CompanyRecord) CacheInfo() CacheInfo { return r.cache }

// UpdatedAtTime parses UpdatedAt strictly as RFC 3339.
func (r *CompanyRecord) UpdatedAtTime() (time.Time, bool) { return parseTime(r.UpdatedAt) }

// UnmarshalJSON decodes with exact key matching.
func (r *CompanyRecord) UnmarshalJSON(b []byte) error {
	v, err := decodeCompanyRecord(b, "")
	if err != nil {
		return err
	}
	*r = *v
	return nil
}

func decodeCompanyRecord(raw json.RawMessage, path string) (*CompanyRecord, error) {
	o, err := parseObject(raw, path)
	if err != nil {
		return nil, err
	}
	r := &CompanyRecord{raw: append(json.RawMessage(nil), raw...)}
	if r.CompanyID, err = o.reqInt64(path, "companyId"); err != nil {
		return nil, err
	}
	if r.Name, err = o.reqString(path, "name"); err != nil {
		return nil, err
	}
	if r.Country, err = o.optString(path, "country"); err != nil {
		return nil, err
	}
	if r.Sector, err = o.optString(path, "sector"); err != nil {
		return nil, err
	}
	if r.Industry, err = o.optString(path, "industry"); err != nil {
		return nil, err
	}
	if r.ISINs, err = o.stringList(path, "isins"); err != nil {
		return nil, err
	}
	if pl, ok := o["primaryListing"]; ok {
		if r.PrimaryListing, err = decodeListing(pl, sub(path, "primaryListing")); err != nil {
			return nil, err
		}
	}
	elems, _, err := o.array(path, "listings")
	if err != nil {
		return nil, err
	}
	r.Listings = make([]Listing, 0, len(elems))
	for i, e := range elems {
		p := fmt.Sprintf("%s[%d]", sub(path, "listings"), i)
		if isJSONNull(e) {
			return nil, decodeErr(p, e, "expected an object, got null")
		}
		l, err := decodeListing(e, p)
		if err != nil {
			return nil, err
		}
		r.Listings = append(r.Listings, *l)
	}
	if r.UpdatedAt, err = o.optString(path, "updatedAt"); err != nil {
		return nil, err
	}
	return r, nil
}

// Source is an entry of /v3/sources. Type is nil when the source is
// unclassified. HasContent is absent without full-article access ("not
// entitled to know", never "no content").
type Source struct {
	Domain     string      `json:"domain"`
	Name       *string     `json:"name"`
	Type       *SourceType `json:"type"`
	IsDefault  bool        `json:"isDefault"`
	IsCustom   bool        `json:"isCustom"`
	Country    *string     `json:"country"`
	Languages  []string    `json:"languages"`
	HasContent Field[bool] `json:"hasContent,omitzero"`

	raw json.RawMessage
}

// RawJSON returns the source exactly as received.
func (s *Source) RawJSON() json.RawMessage { return s.raw }

// UnmarshalJSON decodes with exact key matching.
func (s *Source) UnmarshalJSON(b []byte) error {
	v, err := decodeSource(b, "")
	if err != nil {
		return err
	}
	*s = *v
	return nil
}

func decodeSource(raw json.RawMessage, path string) (*Source, error) {
	o, err := parseObject(raw, path)
	if err != nil {
		return nil, err
	}
	s := &Source{raw: append(json.RawMessage(nil), raw...)}
	if s.Domain, err = o.reqString(path, "domain"); err != nil {
		return nil, err
	}
	if s.Name, err = o.optString(path, "name"); err != nil {
		return nil, err
	}
	t, err := o.optString(path, "type")
	if err != nil {
		return nil, err
	}
	if t != nil {
		st := SourceType(*t)
		s.Type = &st
	}
	for _, f := range []struct {
		key string
		dst *bool
	}{{"isDefault", &s.IsDefault}, {"isCustom", &s.IsCustom}} {
		b, err := o.optBool(path, f.key)
		if err != nil {
			return nil, err
		}
		if b != nil {
			*f.dst = *b
		}
	}
	if s.Country, err = o.optString(path, "country"); err != nil {
		return nil, err
	}
	if s.Languages, err = o.stringList(path, "languages"); err != nil {
		return nil, err
	}
	if hc, ok := o["hasContent"]; ok {
		if isJSONNull(hc) {
			s.HasContent = fieldNullOf[bool]()
		} else {
			b, err := o.optBool(path, "hasContent")
			if err != nil {
				return nil, err
			}
			s.HasContent = fieldOf(*b)
		}
	}
	return s, nil
}

// Pagination is the pagination block of a search page.
type Pagination struct {
	Limit      int     `json:"limit"`
	Returned   int     `json:"returned"`
	HasMore    bool    `json:"hasMore"`
	NextCursor *string `json:"nextCursor"`
}

// SkippedEntry is a list entry that could not be decoded. The rest of the page
// is still delivered; the entry is also logged as a warning.
type SkippedEntry struct {
	Index  int             // position in the list
	Raw    json.RawMessage // the entry as received
	Reason string          // why it was skipped
}

// ArticlePage is one page of articles.search.
type ArticlePage struct {
	Articles   []Article
	Pagination Pagination
	Skipped    []SkippedEntry
}

// CompanyList is the result of companies.search and companies.getMany.
type CompanyList struct {
	Items   []CompanyRecord
	Skipped []SkippedEntry
}

// SourceList is the result of sources.list.
type SourceList struct {
	Items   []Source
	Skipped []SkippedEntry
}

func decodePagination(raw json.RawMessage, path string) (Pagination, error) {
	var p Pagination
	o, err := parseObject(raw, path)
	if err != nil {
		return p, err
	}
	for _, f := range []struct {
		key string
		dst *int
	}{{"limit", &p.Limit}, {"returned", &p.Returned}} {
		n, err := o.optInt64(path, f.key)
		if err != nil {
			return p, err
		}
		if n != nil {
			*f.dst = int(*n)
		}
	}
	hm, err := o.optBool(path, "hasMore")
	if err != nil {
		return p, err
	}
	if hm != nil {
		p.HasMore = *hm
	}
	if p.NextCursor, err = o.optString(path, "nextCursor"); err != nil {
		return p, err
	}
	return p, nil
}

// decodeList decodes each element on its own; broken elements are skipped and
// reported instead of failing the whole page.
func decodeList[T any](elems []json.RawMessage, path string, decode func(json.RawMessage, string) (T, error)) ([]T, []SkippedEntry) {
	out := make([]T, 0, len(elems))
	var skipped []SkippedEntry
	for i, e := range elems {
		v, err := decode(e, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			skipped = append(skipped, SkippedEntry{Index: i, Raw: append(json.RawMessage(nil), e...), Reason: err.Error()})
			continue
		}
		out = append(out, v)
	}
	return out, skipped
}
