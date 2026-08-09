package finlight

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := New(Config{APIKey: "test-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestFetchArticlesSendsAuthAndBody(t *testing.T) {
	var gotBody map[string]any
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/articles" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-key" {
			t.Errorf("X-API-KEY = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "go/finlight-client-go@") {
			t.Errorf("User-Agent = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"status":"ok","page":1,"pageSize":10,"articles":[
			{"link":"https://example.com/a","title":"T","publishDate":"2024-01-01T00:00:00Z",
			 "source":"example.com","language":"en","confidence":"0.9"}]}`))
	})

	resp, err := client.Articles.FetchArticles(context.Background(), GetArticlesParams{
		Query: "nvidia", From: "2024-01-01", PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["query"] != "nvidia" || gotBody["from"] != "2024-01-01" {
		t.Errorf("request body = %v", gotBody)
	}
	if _, ok := gotBody["includeContent"]; ok {
		t.Error("zero-valued includeContent must be omitted")
	}
	if len(resp.Articles) != 1 || resp.Articles[0].Confidence.Float64() != 0.9 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestFetchArticleByLinkUnwrapsEnvelope(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/articles/by-link" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("link") != "https://example.com/a" || q.Get("includeContent") != "true" {
			t.Errorf("query = %v", q)
		}
		if q.Has("includeEntities") {
			t.Error("false includeEntities must be omitted")
		}
		_, _ = w.Write([]byte(`{"article":{"link":"https://example.com/a","title":"T",
			"publishDate":"2024-01-01T00:00:00Z","source":"example.com","language":"en"}}`))
	})

	article, err := client.Articles.FetchArticleByLink(context.Background(), GetArticleByLinkParams{
		Link: "https://example.com/a", IncludeContent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "T" {
		t.Errorf("article = %+v", article)
	}
}

func TestGetSources(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sources" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"domain":"example.com","isContentAvailable":true,"isDefaultSource":false}]`))
	})

	sources, err := client.Sources.GetSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Domain != "example.com" || !sources[0].IsContentAvailable {
		t.Errorf("sources = %+v", sources)
	}
}

func TestRetryOn503ThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})

	if _, err := client.Sources.GetSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestNoRetryOn400(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	})

	_, err := client.Sources.GetSources(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v, want APIError 400", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestRetriesExhausted(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	client, err := New(Config{APIKey: "test-key", BaseURL: srv.URL, RetryCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Sources.GetSources(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want APIError 503", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestContextCancelDuringBackoff(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.Sources.GetSources(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Errorf("cancellation took %v, backoff not interrupted", elapsed)
	}
}
