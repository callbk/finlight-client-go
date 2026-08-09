package finlight

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FlexFloat is a float64 that also unmarshals from JSON strings ("0.95").
// The API and webhooks deliver confidence values in both representations.
type FlexFloat float64

func (f *FlexFloat) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		return nil
	}
	s = strings.Trim(s, `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("finlight: cannot parse %q as float: %w", s, err)
	}
	*f = FlexFloat(v)
	return nil
}

func (f FlexFloat) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(f))
}

// Float64 returns the value as a plain float64.
func (f FlexFloat) Float64() float64 { return float64(f) }

// flexTimeLayouts are tried in order when parsing timestamps. The API mostly
// sends RFC 3339, but some feeds omit the zone or use a space separator.
var flexTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

// FlexTime is a time.Time that unmarshals from the timestamp formats used by
// the finlight API (RFC 3339 with or without zone, space-separated, date-only).
type FlexTime struct {
	time.Time
}

func (t *FlexTime) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		return nil
	}
	s = strings.Trim(s, `"`)
	for _, layout := range flexTimeLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("finlight: cannot parse %q as timestamp", s)
}

func (t FlexTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Time.Format(time.RFC3339Nano))
}

// Listing describes one exchange listing of a company.
type Listing struct {
	Ticker          string `json:"ticker"`
	ExchangeCode    string `json:"exchangeCode"`
	ExchangeCountry string `json:"exchangeCountry"`
}

// Company is an entity recognized in an article.
type Company struct {
	CompanyID      int        `json:"companyId"`
	Confidence     *FlexFloat `json:"confidence,omitempty"`
	Country        string     `json:"country,omitempty"`
	Exchange       string     `json:"exchange,omitempty"`
	Industry       string     `json:"industry,omitempty"`
	Sector         string     `json:"sector,omitempty"`
	Name           string     `json:"name"`
	Ticker         string     `json:"ticker"`
	ISIN           string     `json:"isin,omitempty"`
	OpenFIGI       string     `json:"openfigi,omitempty"`
	PrimaryListing *Listing   `json:"primaryListing,omitempty"`
	ISINs          []string   `json:"isins,omitempty"`
	OtherListings  []Listing  `json:"otherListings,omitempty"`
}

// Article is an enriched news article as returned by the REST API, the
// enhanced WebSocket stream, and webhooks.
type Article struct {
	Link        string     `json:"link"`
	Title       string     `json:"title"`
	PublishDate FlexTime   `json:"publishDate"`
	Source      string     `json:"source"`
	Language    string     `json:"language"`
	Summary     string     `json:"summary,omitempty"`
	Images      []string   `json:"images,omitempty"`
	CreatedAt   *FlexTime  `json:"createdAt,omitempty"`
	RevisedDate *FlexTime  `json:"revisedDate,omitempty"`
	IsUpdate    *bool      `json:"isUpdate,omitempty"`
	Categories  []string   `json:"categories,omitempty"`
	Sentiment   string     `json:"sentiment,omitempty"`
	Confidence  *FlexFloat `json:"confidence,omitempty"`
	Content     string     `json:"content,omitempty"`
	Companies   []Company  `json:"companies,omitempty"`
	Countries   []string   `json:"countries,omitempty"`
}

// RawArticle is an unenriched article as delivered by the raw WebSocket
// stream (no sentiment, entities, or content).
type RawArticle struct {
	Link        string    `json:"link"`
	Title       string    `json:"title"`
	PublishDate FlexTime  `json:"publishDate"`
	Source      string    `json:"source"`
	Language    string    `json:"language"`
	Summary     string    `json:"summary,omitempty"`
	Images      []string  `json:"images,omitempty"`
	CreatedAt   *FlexTime `json:"createdAt,omitempty"`
	RevisedDate *FlexTime `json:"revisedDate,omitempty"`
	IsUpdate    *bool     `json:"isUpdate,omitempty"`
	Categories  []string  `json:"categories,omitempty"`
}

// ArticleResponse is the paginated result of ArticleService.FetchArticles.
type ArticleResponse struct {
	Status   string    `json:"status"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Articles []Article `json:"articles"`
}

// Source is a news source available through the API.
type Source struct {
	Domain             string `json:"domain"`
	IsContentAvailable bool   `json:"isContentAvailable"`
	IsDefaultSource    bool   `json:"isDefaultSource"`
}
