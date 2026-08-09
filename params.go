package finlight

// Category is an article category assigned by finlight's classification.
type Category string

// All categories known to the API.
const (
	CategoryMarkets     Category = "markets"
	CategoryEconomy     Category = "economy"
	CategoryBusiness    Category = "business"
	CategoryPolitics    Category = "politics"
	CategoryGeopolitics Category = "geopolitics"
	CategoryRegulation  Category = "regulation"
	CategoryTechnology  Category = "technology"
	CategoryEnergy      Category = "energy"
	CategoryCommodities Category = "commodities"
	CategoryCrypto      Category = "crypto"
	CategoryHealth      Category = "health"
	CategoryClimate     Category = "climate"
	CategorySecurity    Category = "security"
)

// OrderBy selects the sort field for article queries.
type OrderBy string

const (
	OrderByPublishDate OrderBy = "publishDate"
	OrderByCreatedAt   OrderBy = "createdAt"
	OrderByRevisedDate OrderBy = "revisedDate"
)

// SortOrder selects the sort direction for article queries.
type SortOrder string

const (
	SortOrderAsc  SortOrder = "ASC"
	SortOrderDesc SortOrder = "DESC"
)

// GetArticlesParams are the search parameters for ArticleService.FetchArticles.
// Zero-valued fields are omitted from the request; the server applies its
// documented defaults.
type GetArticlesParams struct {
	// Query supports the finlight query language, e.g.
	// `(ticker:AAPL OR ticker:NVDA) AND NOT source:www.reuters.com AND "Elon Musk"`.
	Query string `json:"query,omitempty"`
	// Deprecated: use Sources.
	Source              string     `json:"source,omitempty"`
	Sources             []string   `json:"sources,omitempty"`
	ExcludeSources      []string   `json:"excludeSources,omitempty"`
	OptInSources        []string   `json:"optInSources,omitempty"`
	From                string     `json:"from,omitempty"` // YYYY-MM-DD or ISO 8601
	To                  string     `json:"to,omitempty"`
	Language            string     `json:"language,omitempty"`
	Tickers             []string   `json:"tickers,omitempty"`
	IncludeEntities     bool       `json:"includeEntities,omitempty"`
	ExcludeEmptyContent bool       `json:"excludeEmptyContent,omitempty"`
	IncludeContent      bool       `json:"includeContent,omitempty"`
	OrderBy             OrderBy    `json:"orderBy,omitempty"`
	Order               SortOrder  `json:"order,omitempty"`
	PageSize            int        `json:"pageSize,omitempty"` // 1–1000
	Page                int        `json:"page,omitempty"`
	Countries           []string   `json:"countries,omitempty"`
	Categories          []Category `json:"categories,omitempty"`
}

// GetArticleByLinkParams are the parameters for ArticleService.FetchArticleByLink.
type GetArticleByLinkParams struct {
	Link            string // required
	IncludeContent  bool
	IncludeEntities bool
}

// GetArticlesWebSocketParams filter the enhanced WebSocket stream.
type GetArticlesWebSocketParams struct {
	Query          string   `json:"query,omitempty"`
	Sources        []string `json:"sources,omitempty"`
	ExcludeSources []string `json:"excludeSources,omitempty"`
	OptInSources   []string `json:"optInSources,omitempty"`
	Language       string   `json:"language,omitempty"`
	// Deprecated: use IncludeContent.
	Extended            bool       `json:"extended,omitempty"`
	Tickers             []string   `json:"tickers,omitempty"`
	IncludeEntities     bool       `json:"includeEntities,omitempty"`
	ExcludeEmptyContent bool       `json:"excludeEmptyContent,omitempty"`
	IncludeContent      bool       `json:"includeContent,omitempty"`
	Countries           []string   `json:"countries,omitempty"`
	Categories          []Category `json:"categories,omitempty"`
	IncludeUpdates      bool       `json:"includeUpdates,omitempty"`
}

// GetRawArticlesWebSocketParams filter the raw WebSocket stream.
type GetRawArticlesWebSocketParams struct {
	Query          string   `json:"query,omitempty"`
	Sources        []string `json:"sources,omitempty"`
	ExcludeSources []string `json:"excludeSources,omitempty"`
	OptInSources   []string `json:"optInSources,omitempty"`
	Language       string   `json:"language,omitempty"`
	IncludeUpdates bool     `json:"includeUpdates,omitempty"`
}
