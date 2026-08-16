# finlight-client-go

*[English](README.md) | 简体中文 | [日本語](README.ja.md) | [한국어](README.ko.md)*

[finlight.me](https://finlight.me) API 的官方 Go 客户端 —— 提供带情感分析、实体识别和实时流式推送的财经新闻。

📚 **完整 API 文档：[docs.finlight.me](https://docs.finlight.me)**

## 功能特性

- **REST API**：检索文章、按链接获取单篇文章、列出新闻源
- **实时流式推送**：通过 WebSocket 提供 enhanced 和 raw 两种文章流，以 Go 1.23 的 range-over-func 迭代器形式暴露
- **默认具备韧性**：请求失败按指数退避重试；WebSocket 自动重连、带 pong 看门狗的保活、主动连接轮换，以及速率限制处理
- **支持 Webhook**：HMAC-SHA256 签名验证，含重放防护
- **依赖极少**：仅依赖 [`github.com/coder/websocket`](https://github.com/coder/websocket)（其本身无依赖）；日志使用标准库 `log/slog`

## 安装

```bash
go get github.com/callbk/finlight-client-go
```

需要 Go 1.23 及以上版本。

## 快速开始

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

### 检索文章

完整的 `Query` 语法参见[查询语言参考](https://docs.finlight.me)。

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

### 按链接获取文章

```go
article, err := client.Articles.FetchArticleByLink(ctx, finlight.GetArticleByLinkParams{
	Link:           "https://example.com/some-article",
	IncludeContent: true,
})
```

### 列出新闻源

```go
sources, err := client.Sources.GetSources(ctx)
```

## WebSocket 流式推送

数据流是 Go 迭代器：用 range 遍历，break 即停止，取消 context 即关闭。重连、保活和速率限制等待均在内部处理。

### Enhanced 流（含情感、实体，已去重）

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
	log.Fatal(err) // 例如 finlight.ErrBlocked
}
```

### Raw 流（延迟最低，无增强处理）

```go
for article := range client.RawWebsocket.Stream(ctx, finlight.GetRawArticlesWebSocketParams{
	Sources: []string{"www.reuters.com"},
}) {
	fmt.Println(article.Title)
}
```

### 自定义 WebSocket 选项

```go
ws := finlight.NewWebSocketClient(cfg, finlight.WebSocketOptions{
	PingInterval:       25 * time.Second,
	PongTimeout:        60 * time.Second,
	BaseReconnectDelay: 500 * time.Millisecond,
	MaxReconnectDelay:  10 * time.Second,
	ConnectionLifetime: 115 * time.Minute,
	Takeover:           true, // 接管该密钥下已有的连接
	OnClose: func(code int, reason string) {
		log.Printf("connection closed: %d %s", code, reason)
	},
})
```

## Webhook

使用 finlight 控制台中的 endpoint secret 验证收到的 Webhook：

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
		r.Header.Get("X-Webhook-Timestamp"), // 缺失时传 ""
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fmt.Println("new article:", article.Title)
	w.WriteHeader(http.StatusOK)
})
```

## 配置

| 字段         | 默认值                    | 说明                                                 |
| ------------ | ------------------------- | ---------------------------------------------------- |
| `APIKey`     | ——（必填）                | 你的 finlight API 密钥                               |
| `BaseURL`    | `https://api.finlight.me` | REST 基础地址                                        |
| `WssURL`     | `wss://wss.finlight.me`   | WebSocket 地址（raw 流会追加 `/raw`）                |
| `Timeout`    | `5s`                      | 单次请求超时                                         |
| `RetryCount` | `3`                       | 请求总尝试次数                                       |
| `Logger`     | 静默                      | `*slog.Logger`；传入 `slog.Default()` 可开启日志     |
| `HTTPClient` | 由 `Timeout` 派生         | 可选的 `*http.Client` 覆盖                           |

WebSocket 选项默认值：每 `25s` 发送一次 ping，pong 超时 `60s`，重连退避 `500ms → 10s`，`115min` 后主动轮换连接。

### 日志

客户端默认静默。传入任意 `*slog.Logger` 即可观察其运行过程：

```go
client, _ := finlight.New(finlight.Config{
	APIKey: apiKey,
	Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
})
```

## 错误处理

- REST：可重试的状态码（429、500、502、503、504）会按指数退避重试；其他失败返回 `*finlight.APIError`（用 `errors.As` 检查）。
- 流式推送：`Stream` 在 context 取消或消费方 break 时静默结束。之后请检查 `Err()` —— `finlight.ErrBlocked` 表示服务端永久拒绝了该连接。
- Webhook：验证失败返回 `*finlight.WebhookVerificationError`。

## 测试

```bash
go test ./...                                     # 单元测试（离线）
FINLIGHT_API_KEY=sk_... go test -tags integration ./...  # 针对线上 API
```

## 许可证

MIT —— 参见 [LICENSE](LICENSE)。

## 支持

- 文档：https://docs.finlight.me
- 官网：https://finlight.me
- 中文产品页：https://finlight.me/zh/news-api
