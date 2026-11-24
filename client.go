package finlight

import (
	"fmt"
	"io"
)

// Client is the finlight API v3 client. It is safe for concurrent use and can
// run several streams at the same time. It holds no connections of its own
// besides streams, so there is nothing to close.
type Client struct {
	// Articles: search, searchAll, get and getByLink.
	Articles *ArticlesService
	// Companies: company master data (needs entityAccess).
	Companies *CompaniesService
	// Sources: the sources of the subscription.
	Sources *SourcesService

	cfg Config // APIKey cleared; the key lives in key only
	key secret
	t   *transport
}

// NewClient builds a client. Only Config.APIKey is required; an empty key
// returns a *ValidationError.
func NewClient(cfg Config) (*Client, error) {
	full, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	key := newSecret(full.APIKey)
	full.APIKey = ""
	c := &Client{cfg: full, key: key, t: newTransport(full, key)}
	c.Articles = &ArticlesService{c: c}
	c.Companies = &CompaniesService{c: c}
	c.Sources = &SourcesService{c: c}
	return c, nil
}

// String never shows the API key. It has a value receiver so that printing a
// Client value (not only a pointer) goes through it too.
func (c Client) String() string {
	return fmt.Sprintf("finlight.Client{BaseURL:%q, WSSURL:%q}", c.cfg.BaseURL, c.cfg.WSSURL)
}

// GoString never shows the API key.
func (c Client) GoString() string { return c.String() }

// Format prints String for every verb, so that no verb reaches the fields.
func (c Client) Format(f fmt.State, verb rune) { formatRedacted(f, verb, c.String()) }

// secret holds the API key. It is a pointer, so that fmt prints an address
// even where it cannot call a method (an unexported field of a struct).
type secret struct{ p *string }

func newSecret(s string) secret { return secret{p: &s} }

func (s secret) reveal() string {
	if s.p == nil {
		return ""
	}
	return *s.p
}

func (s secret) String() string                { return "[redacted]" }
func (s secret) GoString() string              { return "[redacted]" }
func (s secret) Format(f fmt.State, verb rune) { formatRedacted(f, verb, s.String()) }

// formatRedacted writes text for every verb; %q quotes it.
func formatRedacted(f fmt.State, verb rune, text string) {
	if verb == 'q' {
		text = fmt.Sprintf("%q", text)
	}
	_, _ = io.WriteString(f, text)
}
