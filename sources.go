package finlight

import (
	"context"
	"encoding/json"
	"net/http"
)

// SourcesService groups the source endpoint (Client.Sources).
type SourcesService struct{ c *Client }

// List returns the sources of the subscription, sorted by domain. HasContent
// is absent on every source without full-article access. Each successful
// request costs 1 credit; error answers are free.
func (s *SourcesService) List(ctx context.Context) (*SourceList, error) {
	resp, err := s.c.t.do(ctx, apiRequest{method: http.MethodGet, path: "/v3/sources"})
	if err != nil {
		return nil, err
	}
	o, err := parseObject(resp.body, "")
	if err != nil {
		return nil, err
	}
	elems, err := o.envelope("sources")
	if err != nil {
		return nil, err
	}
	items, skipped := decodeList(elems, "sources", func(raw json.RawMessage, path string) (Source, error) {
		v, err := decodeSource(raw, path)
		if err != nil {
			return Source{}, err
		}
		return *v, nil
	})
	s.c.logSkipped("sources", skipped)
	return &SourceList{Items: items, Skipped: skipped}, nil
}
