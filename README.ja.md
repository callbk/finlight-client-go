# finlight-client-go

*[English](README.md) | [简体中文](README.zh-CN.md) | 日本語 | [한국어](README.ko.md)*

[finlight.me](https://finlight.me) API の公式 Go クライアントです。センチメント分析、エンティティ認識、リアルタイムストリーミングを備えた金融ニュースを提供します。

📚 **完全な API ドキュメント: [docs.finlight.me](https://docs.finlight.me)**

## 主な機能

- **REST API**: 記事の検索、リンクによる単一記事の取得、ニュースソースの一覧取得
- **リアルタイムストリーミング**: WebSocket 経由の enhanced および raw 記事ストリームを、Go 1.23 の range-over-func イテレータとして提供
- **標準で堅牢**: 指数バックオフによるリクエストのリトライ、WebSocket の自動再接続、pong ウォッチドッグ付きキープアライブ、先回りのコネクションローテーション、レート制限のハンドリング
- **Webhook 対応**: HMAC-SHA256 の署名検証とリプレイ攻撃対策
- **依存関係は最小限**: [`github.com/coder/websocket`](https://github.com/coder/websocket) のみ（それ自体は依存関係なし）、ロギングは標準ライブラリの `log/slog`

## インストール

```bash
go get github.com/callbk/finlight-client-go
```

Go 1.23 以降が必要です。

## クイックスタート

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

### 記事を検索する

`Query` の完全な構文は[クエリ言語リファレンス](https://docs.finlight.me)を参照してください。

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

### リンクから記事を取得する

```go
article, err := client.Articles.FetchArticleByLink(ctx, finlight.GetArticleByLinkParams{
	Link:           "https://example.com/some-article",
	IncludeContent: true,
})
```

### ニュースソースを一覧取得する

```go
sources, err := client.Sources.GetSources(ctx)
```

## WebSocket ストリーミング

ストリームは Go のイテレータです。range で回し、break で停止し、context をキャンセルすれば終了します。再接続、キープアライブ、レート制限による待機は内部で処理されます。

### Enhanced ストリーム（センチメント、エンティティ、重複除去あり）

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
	log.Fatal(err) // 例: finlight.ErrBlocked
}
```

### Raw ストリーム（最小遅延、エンリッチメントなし）

```go
for article := range client.RawWebsocket.Stream(ctx, finlight.GetRawArticlesWebSocketParams{
	Sources: []string{"www.reuters.com"},
}) {
	fmt.Println(article.Title)
}
```

### WebSocket オプションのカスタマイズ

```go
ws := finlight.NewWebSocketClient(cfg, finlight.WebSocketOptions{
	PingInterval:       25 * time.Second,
	PongTimeout:        60 * time.Second,
	BaseReconnectDelay: 500 * time.Millisecond,
	MaxReconnectDelay:  10 * time.Second,
	ConnectionLifetime: 115 * time.Minute,
	Takeover:           true, // 同じキーの既存コネクションをテイクオーバーする
	OnClose: func(code int, reason string) {
		log.Printf("connection closed: %d %s", code, reason)
	},
})
```

## Webhook

finlight ダッシュボードで取得したエンドポイントシークレットで、受信した Webhook を検証します:

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
		r.Header.Get("X-Webhook-Timestamp"), // 存在しない場合は ""
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fmt.Println("new article:", article.Title)
	w.WriteHeader(http.StatusOK)
})
```

## 設定

| フィールド   | デフォルト                | 説明                                                     |
| ------------ | ------------------------- | -------------------------------------------------------- |
| `APIKey`     | —（必須）                 | あなたの finlight API キー                               |
| `BaseURL`    | `https://api.finlight.me` | REST のベース URL                                        |
| `WssURL`     | `wss://wss.finlight.me`   | WebSocket の URL（raw では `/raw` が付加されます）       |
| `Timeout`    | `5s`                      | リクエストごとのタイムアウト                             |
| `RetryCount` | `3`                       | リクエストの総試行回数                                   |
| `Logger`     | 無効                      | `*slog.Logger`。`slog.Default()` を渡すとログが有効化    |
| `HTTPClient` | `Timeout` から生成        | 任意の `*http.Client` で上書き可能                       |

WebSocket オプションのデフォルト: ping は `25s` ごと、pong タイムアウト `60s`、再接続バックオフ `500ms → 10s`、`115min` 経過後に先回りローテーション。

### ロギング

クライアントはデフォルトでは何も出力しません。任意の `*slog.Logger` を渡すと動作を確認できます:

```go
client, _ := finlight.New(finlight.Config{
	APIKey: apiKey,
	Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
})
```

## エラーハンドリング

- REST: リトライ対象のステータス（429、500、502、503、504）は指数バックオフでリトライされます。それ以外の失敗は `*finlight.APIError` を返します（`errors.As` で判定してください）。
- ストリーミング: `Stream` は context のキャンセルまたは呼び出し側の break で静かに終了します。その後 `Err()` を確認してください。`finlight.ErrBlocked` はサーバーが接続を恒久的に拒否したことを意味します。
- Webhook: 検証失敗時は `*finlight.WebhookVerificationError` を返します。

## テスト

```bash
go test ./...                                     # ユニットテスト（オフライン）
FINLIGHT_API_KEY=sk_... go test -tags integration ./...  # 実 API に対して実行
```

## ライセンス

MIT — [LICENSE](LICENSE) を参照してください。

## サポート

- ドキュメント: https://docs.finlight.me
- ウェブサイト: https://finlight.me
- 日本語の製品ページ: https://finlight.me/ja/news-api
