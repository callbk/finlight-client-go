# finlight-client-go

*[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | 한국어*

[finlight.me](https://finlight.me) API의 공식 Go 클라이언트입니다. 감성 분석, 엔티티 인식, 실시간 스트리밍을 갖춘 금융 뉴스를 제공합니다.

📚 **전체 API 문서: [docs.finlight.me](https://docs.finlight.me)**

## 주요 기능

- **REST API**: 기사 검색, 링크로 단일 기사 조회, 뉴스 소스 목록 조회
- **실시간 스트리밍**: WebSocket을 통한 enhanced 및 raw 기사 스트림을 Go 1.23의 range-over-func 이터레이터로 제공
- **기본적으로 견고함**: 지수 백오프를 적용한 요청 재시도, WebSocket 자동 재연결, pong 워치독 기반 킵얼라이브, 선제적 연결 교체, 속도 제한 처리
- **Webhook 지원**: HMAC-SHA256 서명 검증 및 재생 공격 방지
- **최소한의 의존성**: [`github.com/coder/websocket`](https://github.com/coder/websocket) 하나뿐이며(그 자체는 의존성 없음), 로깅은 표준 라이브러리 `log/slog` 사용

## 설치

```bash
go get github.com/callbk/finlight-client-go
```

Go 1.23 이상이 필요합니다.

## 빠른 시작

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

### 기사 검색

`Query`의 전체 문법은 [쿼리 언어 레퍼런스](https://docs.finlight.me)를 참조하세요.

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

### 링크로 기사 조회

```go
article, err := client.Articles.FetchArticleByLink(ctx, finlight.GetArticleByLinkParams{
	Link:           "https://example.com/some-article",
	IncludeContent: true,
})
```

### 뉴스 소스 목록 조회

```go
sources, err := client.Sources.GetSources(ctx)
```

## WebSocket 스트리밍

스트림은 Go 이터레이터입니다. range로 순회하고, break로 중단하며, context를 취소하면 종료됩니다. 재연결, 킵얼라이브, 속도 제한 대기는 내부에서 처리됩니다.

### Enhanced 스트림(감성, 엔티티, 중복 제거 포함)

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
	log.Fatal(err) // 예: finlight.ErrBlocked
}
```

### Raw 스트림(최저 지연, 보강 없음)

```go
for article := range client.RawWebsocket.Stream(ctx, finlight.GetRawArticlesWebSocketParams{
	Sources: []string{"www.reuters.com"},
}) {
	fmt.Println(article.Title)
}
```

### WebSocket 옵션 사용자 지정

```go
ws := finlight.NewWebSocketClient(cfg, finlight.WebSocketOptions{
	PingInterval:       25 * time.Second,
	PongTimeout:        60 * time.Second,
	BaseReconnectDelay: 500 * time.Millisecond,
	MaxReconnectDelay:  10 * time.Second,
	ConnectionLifetime: 115 * time.Minute,
	Takeover:           true, // 동일한 키의 기존 연결을 테이크오버
	OnClose: func(code int, reason string) {
		log.Printf("connection closed: %d %s", code, reason)
	},
})
```

## Webhook

finlight 대시보드에서 발급받은 엔드포인트 시크릿으로 수신한 Webhook을 검증합니다:

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
		r.Header.Get("X-Webhook-Timestamp"), // 없으면 ""
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fmt.Println("new article:", article.Title)
	w.WriteHeader(http.StatusOK)
})
```

## 설정

| 필드         | 기본값                    | 설명                                                     |
| ------------ | ------------------------- | -------------------------------------------------------- |
| `APIKey`     | —(필수)                   | 사용자의 finlight API 키                                 |
| `BaseURL`    | `https://api.finlight.me` | REST 기본 URL                                            |
| `WssURL`     | `wss://wss.finlight.me`   | WebSocket URL(raw는 `/raw`가 덧붙습니다)                 |
| `Timeout`    | `5s`                      | 요청당 타임아웃                                          |
| `RetryCount` | `3`                       | 요청 총 시도 횟수                                        |
| `Logger`     | 비활성                    | `*slog.Logger`. `slog.Default()`를 넘기면 로그가 켜집니다 |
| `HTTPClient` | `Timeout`에서 파생        | 선택적 `*http.Client` 재정의                             |

WebSocket 옵션 기본값: ping은 `25s`마다, pong 타임아웃 `60s`, 재연결 백오프 `500ms → 10s`, `115min` 후 선제적 교체.

### 로깅

클라이언트는 기본적으로 아무것도 출력하지 않습니다. 임의의 `*slog.Logger`를 넘기면 동작을 확인할 수 있습니다:

```go
client, _ := finlight.New(finlight.Config{
	APIKey: apiKey,
	Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
})
```

## 오류 처리

- REST: 재시도 대상 상태 코드(429, 500, 502, 503, 504)는 지수 백오프로 재시도됩니다. 그 외 실패는 `*finlight.APIError`를 반환합니다(`errors.As`로 확인하세요).
- 스트리밍: `Stream`은 context 취소 또는 소비 측의 break 시 조용히 종료됩니다. 이후 `Err()`를 확인하세요. `finlight.ErrBlocked`는 서버가 연결을 영구적으로 거부했음을 의미합니다.
- Webhook: 검증 실패 시 `*finlight.WebhookVerificationError`를 반환합니다.

## 테스트

```bash
go test ./...                                     # 단위 테스트(오프라인)
FINLIGHT_API_KEY=sk_... go test -tags integration ./...  # 실제 API 대상
```

## 라이선스

MIT — [LICENSE](LICENSE) 참조.

## 지원

- 문서: https://docs.finlight.me
- 웹사이트: https://finlight.me
- 한국어 제품 페이지: https://finlight.me/ko/news-api
