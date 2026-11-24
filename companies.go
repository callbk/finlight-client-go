package finlight

import (
	"container/list"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// CompaniesService groups the company master data endpoints
// (Client.Companies). They need the entityAccess entitlement, else the server
// answers 403 FORBIDDEN. Each request costs 1 credit, like an article request
// (GetMany one per request, however many ids); a 304 revalidation and error
// answers cost 1 credit too.
type CompaniesService struct {
	c     *Client
	cache *companyCache
}

// Search looks companies up by text (ticker, name or ISIN), optionally narrowed
// to one exchange. At most 10 results, no pagination.
func (s *CompaniesService) Search(ctx context.Context, params CompanySearchParams) (*CompanyList, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	q := url.Values{"q": {params.Q}}
	if params.MIC != "" {
		q.Set("mic", params.MIC)
	}
	return s.list(ctx, q)
}

// GetMany fetches 1 to 10 companies by id. Unknown ids are left out and the
// order of the result is not guaranteed.
func (s *CompaniesService) GetMany(ctx context.Context, ids []int64) (*CompanyList, error) {
	if len(ids) < 1 || len(ids) > 10 {
		return nil, validationErr("ids", "ids takes 1 to 10 company ids, got %d", len(ids))
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		if id < 1 {
			return nil, validationErr("ids", "company ids start at 1, got %d", id)
		}
		parts[i] = strconv.FormatInt(id, 10)
	}
	return s.list(ctx, url.Values{"ids": {strings.Join(parts, ",")}})
}

func (s *CompaniesService) list(ctx context.Context, q url.Values) (*CompanyList, error) {
	resp, err := s.c.t.do(ctx, apiRequest{method: http.MethodGet, path: "/v3/companies", query: q})
	if err != nil {
		return nil, err
	}
	o, err := parseObject(resp.body, "")
	if err != nil {
		return nil, err
	}
	elems, err := o.envelope("companies")
	if err != nil {
		return nil, err
	}
	items, skipped := decodeList(elems, "companies", func(raw json.RawMessage, path string) (CompanyRecord, error) {
		r, err := decodeCompanyRecord(raw, path)
		if err != nil {
			return CompanyRecord{}, err
		}
		return *r, nil
	})
	s.c.logSkipped("companies", skipped)
	return &CompanyList{Items: items, Skipped: skipped}, nil
}

// Get fetches one company with its ETag and Cache-Control (see
// CompanyRecord.CacheInfo). With opts.IfNoneMatch set and an unchanged record,
// the server answers 304 and Get returns (nil, nil). Without it, the SDK
// revalidates its own cached copy and returns that copy on a 304. Every call
// costs 1 credit, a 304 included.
func (s *CompaniesService) Get(ctx context.Context, companyID int64, opts *CompanyGetOptions) (*CompanyRecord, error) {
	if companyID < 1 {
		return nil, validationErr("companyId", "companyId starts at 1, got %d", companyID)
	}
	header := http.Header{}
	var cached *CompanyRecord
	callerETag := opts != nil && opts.IfNoneMatch != ""
	if callerETag {
		header.Set("If-None-Match", opts.IfNoneMatch)
	} else if cached = s.cache.get(companyID); cached != nil {
		header.Set("If-None-Match", cached.cache.ETag)
	}
	resp, err := s.c.t.do(ctx, apiRequest{
		method: http.MethodGet,
		path:   "/v3/companies/" + strconv.FormatInt(companyID, 10),
		header: header,
	})
	if err != nil {
		return nil, err
	}
	if resp.status == http.StatusNotModified {
		if callerETag || cached == nil {
			return nil, nil
		}
		out := *cached
		out.cache.NotModified = true
		if et := resp.header.Get("ETag"); et != "" {
			out.cache.ETag = et
		}
		if cc := resp.header.Get("Cache-Control"); cc != "" {
			out.cache.CacheControl = cc
		}
		return &out, nil
	}
	o, err := parseObject(resp.body, "")
	if err != nil {
		return nil, err
	}
	raw, ok := o["company"]
	if !ok {
		return nil, decodeErr("company", nil, "required key is missing")
	}
	rec, err := decodeCompanyRecord(raw, "company")
	if err != nil {
		return nil, err
	}
	rec.cache = CacheInfo{ETag: resp.header.Get("ETag"), CacheControl: resp.header.Get("Cache-Control")}
	if rec.cache.ETag != "" {
		s.cache.put(companyID, rec)
	}
	return rec, nil
}

// companyCache is a small LRU of company records by id, for ETag revalidation.
type companyCache struct {
	mu    sync.Mutex
	size  int
	order *list.List
	items map[int64]*list.Element
}

type cacheEntry struct {
	id  int64
	rec CompanyRecord
}

func newCompanyCache(size int) *companyCache {
	return &companyCache{size: size, order: list.New(), items: map[int64]*list.Element{}}
}

func (c *companyCache) get(id int64) *CompanyRecord {
	if c == nil || c.size == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return nil
	}
	c.order.MoveToFront(el)
	rec := el.Value.(*cacheEntry).rec
	return &rec
}

func (c *companyCache) put(id int64, rec *CompanyRecord) {
	if c == nil || c.size == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[id]; ok {
		el.Value.(*cacheEntry).rec = *rec
		c.order.MoveToFront(el)
		return
	}
	c.items[id] = c.order.PushFront(&cacheEntry{id: id, rec: *rec})
	for c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).id)
	}
}
