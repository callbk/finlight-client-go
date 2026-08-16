# finlight-client-go

*English | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md)*

The official Go client for the [finlight.me](https://finlight.me) API — financial news with sentiment analysis, entity recognition, and real-time streaming.

📚 **Full API documentation: [docs.finlight.me](https://docs.finlight.me)**

## Features

- **REST API**: search articles, fetch single articles by link, list sources
- **Real-time streaming**: enhanced and raw article streams over WebSocket, exposed as Go 1.23 range-over-func iterators
- **Resilient by default**: request retries with exponential backoff; WebSocket auto-reconnect, keepalive with pong watchdog, proactive connection rotation, and rate-limit handling
- **Webhook support**: HMAC-SHA256 signature verification with replay protection
- **Minimal dependencies**: only [`github.com/coder/websocket`](https://github.com/coder/websocket) (itself dependency-free); logging via stdlib `log/slog`

## Installation

```bash
go get github.com/callbk/finlight-client-go
```

Requires Go 1.23+.

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	finlight "github.com/callbk/finlight-client-go"
)

func main() {
	client, err := finlight.New(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Articles.FetchArticles(context.Background(), finlight.GetArticlesParams{
		Query:    "nvidia",
		PageSize: 10,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, article := range resp.Articles {
		fmt.Printf("[%s] %s\n", article.Source, article.Title)
	}
}
```

## REST API

### Search articles

See the [query language reference](https://docs.finlight.me) for the full `Query` syntax.

```go
resp, err := client.Articles.FetchArticles(ctx, finlight.GetArticlesParams{
	Query:           `(ticker:AAPL OR ticker:NVDA) AND "Elon Musk"`,
	From:            "2024-01-01",
	To:              "2024-12-31",
	Language:        "en",
	Categories:      []finlight.Category{finlight.CategoryTechnology, finlight.CategoryMarkets},
	IncludeContent:  true,
	IncludeEntities: true,
	OrderBy:         finlight.OrderByPublishDate,
	Order:           finlight.SortOrderDesc,
	PageSize:        50,
})
```

### Fetch an article by link

```go
article, err := client.Articles.FetchArticleByLink(ctx, finlight.GetArticleByLinkParams{
	Link:           "https://example.com/some-article",
	IncludeContent: true,
})
```

### List sources

```go
sources, err := client.Sources.GetSources(ctx)
```

## WebSocket Streaming

Streams are Go iterators: range over them, break to stop, cancel the context to shut down. Reconnects, keepalive, and rate-limit waits are handled internally.

### Enhanced stream (sentiment, entities, deduplicated)

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

for article := range client.Websocket.Stream(ctx, finlight.GetArticlesWebSocketParams{
	Tickers:        []string{"AAPL", "NVDA"},
	IncludeContent: true,
}) {
	fmt.Printf("%s: %s (sentiment: %s)\n", article.Source, article.Title, article.Sentiment)
}
if err := client.Websocket.Err(); err != nil {
	log.Fatal(err) // e.g. finlight.ErrBlocked
}
```

### Raw stream (lowest latency, no enrichment)

```go
for article := range client.RawWebsocket.Stream(ctx, finlight.GetRawArticlesWebSocketParams{
	Sources: []string{"www.reuters.com"},
}) {
	fmt.Println(article.Title)
}
```

### Custom WebSocket options

```go
ws := finlight.NewWebSocketClient(cfg, finlight.WebSocketOptions{
	PingInterval:       25 * time.Second,
	PongTimeout:        60 * time.Second,
	BaseReconnectDelay: 500 * time.Millisecond,
	MaxReconnectDelay:  10 * time.Second,
	ConnectionLifetime: 115 * time.Minute,
	Takeover:           true, // take over an existing connection for this key
	OnClose: func(code int, reason string) {
		log.Printf("connection closed: %d %s", code, reason)
	},
})
```

## Webhooks

Verify incoming webhooks with your endpoint secret from the finlight dashboard:

```go
http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	article, err := finlight.ConstructWebhookEvent(
		body,
		r.Header.Get("X-Webhook-Signature"),
		os.Getenv("FINLIGHT_WEBHOOK_SECRET"),
		r.Header.Get("X-Webhook-Timestamp"), // "" if absent
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fmt.Println("new article:", article.Title)
	w.WriteHeader(http.StatusOK)
})
```

## Configuration

| Field        | Default                   | Description                                          |
| ------------ | ------------------------- | ---------------------------------------------------- |
| `APIKey`     | — (required)              | Your finlight API key                                |
| `BaseURL`    | `https://api.finlight.me` | REST base URL                                        |
| `WssURL`     | `wss://wss.finlight.me`   | WebSocket URL (`/raw` appended for raw)              |
| `Timeout`    | `5s`                      | Per-request timeout                                  |
| `RetryCount` | `3`                       | Total request attempts                               |
| `Logger`     | silent                    | `*slog.Logger`; pass `slog.Default()` to enable logs |
| `HTTPClient` | derived from `Timeout`    | Optional `*http.Client` override                     |

WebSocket option defaults: ping every `25s`, pong timeout `60s`, reconnect backoff `500ms → 10s`, proactive rotation after `115min`.

### Logging

The client is silent by default. Pass any `*slog.Logger` to see what it does:

```go
client, _ := finlight.New(finlight.Config{
	APIKey: apiKey,
	Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
})
```

## Error Handling

- REST: retryable statuses (429, 500, 502, 503, 504) are retried with exponential backoff; other failures return `*finlight.APIError` (inspect with `errors.As`).
- Streaming: `Stream` ends silently on context cancellation or consumer break. Check `Err()` afterwards — `finlight.ErrBlocked` means the server permanently rejected the connection.
- Webhooks: verification failures return `*finlight.WebhookVerificationError`.

## Testing

```bash
go test ./...                                     # unit tests (offline)
FINLIGHT_API_KEY=sk_... go test -tags integration ./...  # against the live API
```

## License

MIT — see [LICENSE](LICENSE).

## Support

- Documentation: https://docs.finlight.me
- Website: https://finlight.me
