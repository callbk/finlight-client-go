package finlight

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CompaniesService groups the company master data endpoints
// (Client.Companies). They need the entityAccess entitlement, else the server
// answers 403 FORBIDDEN. Each request costs 1 credit, like an article request
// (GetMany one per request, however many ids); a 304 revalidation and error
// answers cost 1 credit too.
type CompaniesService struct {
	c *Client
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
// CompanyRecord.CacheInfo). The SDK keeps no cache of company records: it sends
// If-None-Match only when opts.IfNoneMatch is set, and on a 304 (record
// unchanged) Get returns (nil, nil). Every call costs 1 credit, a 304 included,
// so to save credits keep the records yourself and do not call again within
// the Cache-Control max-age.
func (s *CompaniesService) Get(ctx context.Context, companyID int64, opts *CompanyGetOptions) (*CompanyRecord, error) {
	if companyID < 1 {
		return nil, validationErr("companyId", "companyId starts at 1, got %d", companyID)
	}
	header := http.Header{}
	if opts != nil && opts.IfNoneMatch != "" {
		header.Set("If-None-Match", opts.IfNoneMatch)
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
		return nil, nil
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
	return rec, nil
}
