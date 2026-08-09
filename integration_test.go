//go:build integration

package finlight

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integration tests run against the live API. They are skipped unless
// FINLIGHT_API_KEY is set:
//
//	FINLIGHT_API_KEY=sk_... go test -tags integration ./...
func integrationClient(t *testing.T) *Client {
	t.Helper()
	apiKey := os.Getenv("FINLIGHT_API_KEY")
	if apiKey == "" {
		t.Skip("FINLIGHT_API_KEY not set")
	}
	cfg := Config{APIKey: apiKey}
	if v := os.Getenv("FINLIGHT_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("FINLIGHT_WSS_URL"); v != "" {
		cfg.WssURL = v
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestIntegrationFetchArticles(t *testing.T) {
	client := integrationClient(t)
	resp, err := client.Articles.FetchArticles(context.Background(), GetArticlesParams{
		Query: "nvidia", PageSize: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" && resp.Status != "success" {
		t.Errorf("status = %q", resp.Status)
	}
	if len(resp.Articles) == 0 {
		t.Fatal("no articles returned")
	}
	first := resp.Articles[0]
	if first.Link == "" || first.Title == "" || first.PublishDate.IsZero() {
		t.Errorf("incomplete article: %+v", first)
	}
	t.Logf("fetched %d articles, first: %s", len(resp.Articles), first.Title)
}

func TestIntegrationFetchArticleByLink(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()
	resp, err := client.Articles.FetchArticles(ctx, GetArticlesParams{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Articles) == 0 {
		t.Skip("no article available to look up")
	}
	article, err := client.Articles.FetchArticleByLink(ctx, GetArticleByLinkParams{
		Link: resp.Articles[0].Link,
	})
	if err != nil {
		t.Fatal(err)
	}
	if article.Link != resp.Articles[0].Link {
		t.Errorf("link = %q, want %q", article.Link, resp.Articles[0].Link)
	}
}

func TestIntegrationGetSources(t *testing.T) {
	client := integrationClient(t)
	sources, err := client.Sources.GetSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no sources returned")
	}
	t.Logf("%d sources", len(sources))
}

func TestIntegrationWebSocketConnects(t *testing.T) {
	client := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// A successful admit without an error until the deadline counts as a pass;
	// receiving an article within the window is a bonus, not a requirement.
	count := 0
	for range client.Websocket.Stream(ctx, GetArticlesWebSocketParams{}) {
		count++
		break
	}
	if err := client.Websocket.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("received %d article(s) within window", count)
}
