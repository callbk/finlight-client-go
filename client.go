package finlight

import "fmt"

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

	cfg Config
	t   *transport
}

// NewClient builds a client. Only Config.APIKey is required; an empty key
// returns a *ValidationError.
func NewClient(cfg Config) (*Client, error) {
	full, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	c := &Client{cfg: full, t: newTransport(full)}
	c.Articles = &ArticlesService{c: c}
	c.Companies = &CompaniesService{c: c, cache: newCompanyCache(full.CompanyCacheSize)}
	c.Sources = &SourcesService{c: c}
	return c, nil
}

// String never shows the API key.
func (c *Client) String() string {
	return fmt.Sprintf("finlight.Client{BaseURL:%q, WSSURL:%q}", c.cfg.BaseURL, c.cfg.WSSURL)
}

// GoString never shows the API key.
func (c *Client) GoString() string { return c.String() }
