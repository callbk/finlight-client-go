package finlight

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ArticlesService groups the article endpoints (Client.Articles).
type ArticlesService struct{ c *Client }

// Search fetches one page of articles. Every call costs quota, also a failed one.
func (s *ArticlesService) Search(ctx context.Context, params SearchParams) (*ArticlePage, error) {
	body, err := params.body()
	if err != nil {
		return nil, err
	}
	data, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	return s.c.searchPage(ctx, data)
}

func (c *Client) searchPage(ctx context.Context, body []byte) (*ArticlePage, error) {
	resp, err := c.t.do(ctx, apiRequest{method: http.MethodPost, path: "/v3/articles/search", body: body})
	if err != nil {
		return nil, err
	}
	o, err := parseObject(resp.body, "")
	if err != nil {
		return nil, err
	}
	elems, err := o.envelope("articles")
	if err != nil {
		return nil, err
	}
	articles, skipped := decodeList(elems, "articles", func(raw json.RawMessage, path string) (Article, error) {
		a, err := decodeArticle(raw, path)
		if err != nil {
			return Article{}, err
		}
		return *a, nil
	})
	c.logSkipped("articles", skipped)
	page := &ArticlePage{Articles: articles, Skipped: skipped}
	if p, ok := o["pagination"]; ok {
		if page.Pagination, err = decodePagination(p, "pagination"); err != nil {
			return nil, err
		}
	}
	return page, nil
}

func (c *Client) logSkipped(list string, skipped []SkippedEntry) {
	for _, s := range skipped {
		c.cfg.Logger.Warn("finlight: skipped an entry that could not be decoded", "list", list, "index", s.Index, "reason", s.Reason)
	}
}

// Get fetches one article by id. An unknown id is a NOT_FOUND *APIError
// (articles indexed before ids were introduced are reachable by link only).
func (s *ArticlesService) Get(ctx context.Context, id string, opts *GetOptions) (*Article, error) {
	if strings.TrimSpace(id) == "" {
		return nil, validationErr("id", "id must not be empty")
	}
	q, err := includeQuery(opts)
	if err != nil {
		return nil, err
	}
	return s.c.getArticle(ctx, "/v3/articles/"+url.PathEscape(id), q)
}

// GetByLink fetches one article by its link, which must equal the delivered
// link exactly.
func (s *ArticlesService) GetByLink(ctx context.Context, link string, opts *GetOptions) (*Article, error) {
	if link == "" {
		return nil, validationErr("link", "link must not be empty")
	}
	q, err := includeQuery(opts)
	if err != nil {
		return nil, err
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("link", link)
	return s.c.getArticle(ctx, "/v3/articles/by-link", q)
}

func includeQuery(opts *GetOptions) (url.Values, error) {
	if opts == nil || len(opts.Include) == 0 {
		return nil, nil
	}
	if err := checkInclude(opts.Include); err != nil {
		return nil, err
	}
	return url.Values{"include": {strings.Join(toStrings(opts.Include), ",")}}, nil
}

func (c *Client) getArticle(ctx context.Context, path string, q url.Values) (*Article, error) {
	resp, err := c.t.do(ctx, apiRequest{method: http.MethodGet, path: path, query: q})
	if err != nil {
		return nil, err
	}
	o, err := parseObject(resp.body, "")
	if err != nil {
		return nil, err
	}
	raw, ok := o["article"]
	if !ok {
		return nil, decodeErr("article", nil, "required key is missing")
	}
	return decodeArticle(raw, "article")
}

// SearchAll walks the cursor of a search. The body is frozen at this call:
// later pages send the same body with only the cursor swapped. Parameters are
// checked now; requests run while the result is iterated. Each page costs
// quota; MaxPages (default Config.MaxPages = 10) caps the walk.
func (s *ArticlesService) SearchAll(ctx context.Context, params SearchParams, opts *SearchAllOptions) (*SearchAllResult, error) {
	body, err := params.body()
	if err != nil {
		return nil, err
	}
	maxPages := s.c.cfg.MaxPages
	if opts != nil && opts.MaxPages > 0 {
		maxPages = opts.MaxPages
	}
	if opts != nil && opts.MaxPages < 0 {
		return nil, validationErr("maxPages", "maxPages must be positive")
	}
	return &SearchAllResult{c: s.c, ctx: ctx, body: body, maxPages: maxPages}, nil
}

// SearchAllResult is the lazy result of Articles.SearchAll. It can be iterated
// once, either by article (All) or by page (Pages); a second iteration yields
// a *ValidationError instead of searching again.
type SearchAllResult struct {
	c        *Client
	ctx      context.Context
	body     map[string]any
	maxPages int

	mu         sync.Mutex
	started    bool
	nextCursor *string
	truncated  bool
	pages      int
}

// All yields every article of every page. An error ends the iteration as the
// last pair (Article{}, err).
func (r *SearchAllResult) All() iter.Seq2[Article, error] {
	return func(yield func(Article, error) bool) {
		for page, err := range r.Pages() {
			if err != nil {
				yield(Article{}, err)
				return
			}
			for _, a := range page.Articles {
				if !yield(a, nil) {
					return
				}
			}
		}
	}
}

// Pages yields every page. An error ends the iteration as the last pair
// (nil, err).
func (r *SearchAllResult) Pages() iter.Seq2[*ArticlePage, error] {
	return func(yield func(*ArticlePage, error) bool) {
		r.mu.Lock()
		if r.started {
			r.mu.Unlock()
			yield(nil, validationErr("", "a SearchAll result can be iterated only once; call SearchAll again"))
			return
		}
		r.started = true
		r.mu.Unlock()

		var cursor *string
		for {
			if r.PagesFetched() >= r.maxPages {
				r.mu.Lock()
				r.truncated = true
				r.mu.Unlock()
				r.c.cfg.Logger.Warn("finlight: SearchAll stopped at maxPages; continue with NextCursor", "maxPages", r.maxPages)
				return
			}
			body := maps.Clone(r.body)
			if cursor != nil {
				body["cursor"] = *cursor
			}
			data, err := marshalBody(body)
			if err != nil {
				yield(nil, err)
				return
			}
			page, err := r.c.searchPage(r.ctx, data)
			if err != nil {
				yield(nil, err)
				return
			}
			next := page.Pagination.NextCursor
			r.mu.Lock()
			r.pages++
			r.nextCursor = next
			r.mu.Unlock()
			more := page.Pagination.HasMore && next != nil
			if !yield(page, nil) || !more {
				return
			}
			cursor = next
		}
	}
}

// NextCursor is the nextCursor of the last fetched page (nil at the end).
// After a walk stopped by MaxPages, pass it as SearchParams.Cursor to go on.
func (r *SearchAllResult) NextCursor() *string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nextCursor
}

// Truncated reports whether the walk stopped at MaxPages with more pages left.
func (r *SearchAllResult) Truncated() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.truncated
}

// PagesFetched is the number of pages fetched so far.
func (r *SearchAllResult) PagesFetched() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pages
}
