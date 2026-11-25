//go:build integration

package finlight_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	finlight "github.com/callbk/finlight-client-go/v2"
)

// Live tests: dev only, small load, run with the owner's okay:
//
//	FINLIGHT_API_KEY=... FINLIGHT_BASE_URL=https://api.dev.finlight.me \
//	FINLIGHT_WSS_URL=wss://wss.dev.finlight.me go test -tags integration -run Live -v .
//
// Without all three variables every test is skipped; a production host is refused.
func liveClient(t *testing.T) *finlight.Client {
	t.Helper()
	key, base, wss := os.Getenv("FINLIGHT_API_KEY"), os.Getenv("FINLIGHT_BASE_URL"), os.Getenv("FINLIGHT_WSS_URL")
	if key == "" || base == "" || wss == "" {
		t.Skip("FINLIGHT_API_KEY, FINLIGHT_BASE_URL and FINLIGHT_WSS_URL are required for live tests")
	}
	for _, raw := range []string{base, wss} {
		u, err := url.Parse(raw)
		if err != nil || !strings.Contains(u.Hostname(), ".dev.") {
			t.Fatalf("live tests run against dev only, refusing %q", raw)
		}
	}
	c, err := finlight.NewClient(finlight.Config{APIKey: key, BaseURL: base, WSSURL: wss})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveREST(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	page, err := c.Articles.Search(ctx, finlight.SearchParams{Query: "ticker:NVDA", Limit: 3,
		Include: []finlight.IncludeBlock{finlight.IncludeBlockCompanies, finlight.IncludeBlockEvents}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("search: %d articles, skipped %d, hasMore %v", len(page.Articles), len(page.Skipped), page.Pagination.HasMore)
	if len(page.Skipped) != 0 {
		t.Errorf("skipped entries: %+v", page.Skipped)
	}

	res, err := c.Articles.SearchAll(ctx, finlight.SearchParams{Limit: 2}, &finlight.SearchAllOptions{MaxPages: 2})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, err := range res.All() {
		if err != nil {
			t.Fatalf("searchAll: %v", err)
		}
		n++
	}
	t.Logf("searchAll: %d articles on %d pages, truncated %v", n, res.PagesFetched(), res.Truncated())
	if res.PagesFetched() != 2 || !res.Truncated() || res.NextCursor() == nil {
		t.Errorf("pager: pages %d truncated %v", res.PagesFetched(), res.Truncated())
	}

	if len(page.Articles) > 0 {
		a := page.Articles[0]
		byLink, err := c.Articles.GetByLink(ctx, a.Link, &finlight.GetOptions{Include: []finlight.IncludeBlock{finlight.IncludeBlockContent}})
		if err != nil {
			t.Fatalf("getByLink: %v", err)
		}
		t.Logf("getByLink: id %s, content present %v", byLink.ID, byLink.ContentPresent())
		byID, err := c.Articles.Get(ctx, a.ID, nil)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if byID.Link != a.Link {
			t.Errorf("get by id returned %s", byID.Link)
		}
	}
	_, err = c.Articles.Get(ctx, "AAAAAAAAAAAAAAAAAAAAAA", nil)
	var apiErr *finlight.APIError
	if !errors.As(err, &apiErr) || apiErr.APIKind != finlight.APIKindNotFound || apiErr.RequestID == "" {
		t.Errorf("unknown id: %v", err)
	}

	companies, err := c.Companies.Search(ctx, finlight.CompanySearchParams{Q: "NVDA"})
	switch {
	case errors.As(err, &apiErr) && apiErr.APIKind == finlight.APIKindForbidden:
		t.Log("companies: key without entityAccess")
	case err != nil:
		t.Fatalf("companies.search: %v", err)
	case len(companies.Items) > 0:
		id := companies.Items[0].CompanyID
		rec, err := c.Companies.Get(ctx, id, nil)
		if err != nil {
			t.Fatalf("companies.get: %v", err)
		}
		t.Logf("companies: %d results, etag %q", len(companies.Items), rec.CacheInfo().ETag)
		if rec.CacheInfo().ETag != "" {
			nm, err := c.Companies.Get(ctx, id, &finlight.CompanyGetOptions{IfNoneMatch: rec.CacheInfo().ETag})
			if err != nil || nm != nil {
				t.Errorf("If-None-Match: %v, %v", nm, err)
			}
		}
	}

	sources, err := c.Sources.List(ctx)
	if err != nil {
		t.Fatalf("sources: %v", err)
	}
	t.Logf("sources: %d, skipped %d", len(sources.Items), len(sources.Skipped))
}

func TestLiveStream(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admitted := make(chan finlight.AdmittedInfo, 4)
	s, err := c.Stream(ctx, finlight.StreamParams{Query: "ticker:NVDA", Include: []finlight.IncludeBlock{finlight.IncludeBlockCompanies}},
		&finlight.StreamOptions{OnAdmitted: func(a finlight.AdmittedInfo) { admitted <- a }})
	if err != nil {
		t.Fatal(err)
	}
	items := 0
	go func() {
		select {
		case a := <-admitted:
			t.Logf("admitted, lease %s", a.LeaseID)
			// Give the initial push a moment, then stop.
			time.Sleep(5 * time.Second)
		case <-ctx.Done():
		}
		_ = s.Close()
	}()
	for ev, err := range s.Events() {
		if err != nil {
			t.Fatalf("stream ended: %v", err)
		}
		if ev.Kind != finlight.StreamEventKindOther {
			items++
			t.Logf("%s: %s (companies present %v)", ev.Kind, ev.Data.Title, ev.Data.CompaniesPresent())
		}
	}
	if ctx.Err() != nil {
		t.Fatal("no admitted frame within 30 s")
	}
	t.Logf("stream: %d items, closed %v", items, s.Closed())
}
