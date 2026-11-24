# finlight client for Go

*[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | 한국어*

> 이 문서는 [README.md](README.md)의 번역본이며 업데이트가 늦을 수 있습니다. 내용이 다르면 영어판이 우선합니다.

> **Breaking release.** 모듈 `github.com/callbk/finlight-client-go/v2`(v2.0.0)는 finlight API v3만 지원합니다.
> v1용 코드는 그대로 컴파일되지 않습니다. v1(API v2)을 계속 쓰려면 `github.com/callbk/finlight-client-go`
> (`/v2` 없이)를 그대로 import하세요. [v1에서 마이그레이션](#v1에서-마이그레이션)을 참고하세요.

[finlight](https://finlight.me) 뉴스 API의 Go 클라이언트입니다. REST로 금융 뉴스, 기업, 소스를 검색·조회하고,
WebSocket으로 기사를 실시간으로 받으며, Webhook을 검증합니다.

## 버전

| SDK version | API version | Status |
|---|---|---|
| v2.x (`github.com/callbk/finlight-client-go/v2`) | v3 (REST `/v3`, WebSocket `payloadVersion` 3, v3 Webhook) | 현재 |
| v1.x (`github.com/callbk/finlight-client-go`) | v2 | 추가 릴리스 없음 |

SDK 버전과 API 버전은 서로 독립적입니다. API v3를 지원하는 finlight SDK: TypeScript 4.x, Python 3.x,
PHP 2.x, Go 모듈 `/v2`, .NET 2.x, Java 1.x, Ruby 1.x, Rust 0.2.

## 요구 사항

- Go 1.24 이상
- `github.com/coder/websocket` v1.8.13 이상(유일한 의존성)

## 설치

```bash
go get github.com/callbk/finlight-client-go/v2@v2
```

## 빠른 시작

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func main() {
	client, err := finlight.NewClient(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
	if err != nil {
		log.Fatal(err)
	}
	page, err := client.Articles.Search(context.Background(), finlight.SearchParams{Query: "ticker:NVDA", Limit: 10})
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range page.Articles {
		fmt.Println(a.Title)
	}
}
```

모든 호출은 첫 번째 인자로 `context.Context`를 받습니다. 파라미터 구조체의 각 필드는 API 키 하나에
대응하며, 설정한 필드만 전송됩니다. 빈 문자열, nil 슬라이스, nil 포인터는 "설정 안 함"을 뜻합니다.

## 기사 검색

```go
rel := 0.5
page, err := client.Articles.Search(ctx, finlight.SearchParams{
	Query:        "eventType:mna AND companyCountry:US",
	Languages:    []string{"en", "de"},
	From:         time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05.000Z"),
	EventTypes:   []finlight.EventType{finlight.EventTypeMNA},
	MinRelevance: &rel,
	OrderBy:      finlight.OrderByAvailableAt,
	Include:      []finlight.IncludeBlock{finlight.IncludeBlockContent, finlight.IncludeBlockCompanies, finlight.IncludeBlockEvents},
	Limit:        50,
})
fmt.Println(page.Pagination.HasMore, page.Pagination.NextCursor)
```

`From`과 `To`는 주어진 그대로 전송됩니다. 밀리초가 포함된 UTC나 날짜(`2026-11-01`)를 보내세요.
`SearchAll`은 페이지를 자동으로 넘깁니다. 호출 시점에 요청 본문이 고정되고, 이후 페이지에는 cursor만
추가됩니다. 페이지마다 쿼터가 들기 때문에 `MaxPages` 페이지(기본 10) 후에 멈춥니다.

```go
res, err := client.Articles.SearchAll(ctx, finlight.SearchParams{Query: "ticker:AAPL", Limit: 100},
	&finlight.SearchAllOptions{MaxPages: 5})
if err != nil {
	log.Fatal(err) // a parameter was rejected locally
}
for a, err := range res.All() { // or: for page, err := range res.Pages()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(a.ID, a.Title)
}
if res.Truncated() {
	fmt.Println("continue with cursor", *res.NextCursor())
}
```

이어서 가져오려면 같은 필터로 `Cursor: *res.NextCursor()`를 전달하세요. 결과는 한 번만 순회할 수 있으며,
두 번째 순회는 `*finlight.ValidationError`가 됩니다. 형태 오류(예: `Limit: 101`, `Languages: []string{}`)는
전송 전에 `*finlight.ValidationError`로 실패합니다. 키 집합은 구조체 타입으로 고정됩니다. 어휘 값은 로컬에서
검사하지 않으므로 서버의 새 값은 SDK 업데이트 없이 동작합니다.

## 기사, 기업, 소스 조회

```go
article, err := client.Articles.Get(ctx, "EU8JwpsZ8iJehuyPZjMqRz",
	&finlight.GetOptions{Include: []finlight.IncludeBlock{finlight.IncludeBlockContent}})
article, err = client.Articles.GetByLink(ctx, "https://www.reuters.com/...", nil)

companies, err := client.Companies.Search(ctx, finlight.CompanySearchParams{Q: "NVDA", MIC: "XNAS"}) // at most 10, no pages
companies, err = client.Companies.GetMany(ctx, []int64{4821, 1001})
company, err := client.Companies.Get(ctx, 4821, nil) // revalidated with its ETag next time
fmt.Println(company.CacheInfo().ETag, company.CacheInfo().CacheControl)

sources, err := client.Sources.List(ctx)
```

- 찾지 못하면 `APIKind`가 `not_found`인 `*finlight.APIError`가 되며, 빈 결과가 되지 않습니다.
- 기사 ID 도입 전에 색인된 기사는 링크로만 가져올 수 있습니다.
- 기업 조회에는 `entityAccess` 권한이 필요합니다(없으면 `APIKind`가 `forbidden`).
- `Companies.Get`은 최근 레코드를 보관하고 ETag로 재검증합니다(`CompanyCacheSize`).
  직접 `&finlight.CompanyGetOptions{IfNoneMatch: etag}`를 쓰면 304 응답은 `(nil, nil)`을 반환합니다.
  `CacheInfo()`는 `ETag`, `CacheControl`, `NotModified`를 제공합니다.
- `Companies.Search`, `Companies.GetMany`, `Sources.List`는 `Items`와 `Skipped`를 반환합니다.
  검색 페이지의 `ArticlePage.Skipped`도 같습니다. 읽을 수 없는 항목은 여기에 들어갑니다.

## 없음, null, 빈 값

일부 키는 요금제나 `include`에 따라 달라지며 최대 세 가지 상태가 있습니다.

| 상태 | 의미 | 확인 방법 |
|---|---|---|
| 없음 | 요청하지 않음, 권한 없음, 또는 raw 스트림 | `a.ContentPresent()`가 false |
| null | 요청했고 권한도 있지만 값이 없음 | `a.ContentPresent()`가 true이고 `a.Content.IsNull()` |
| 값 | 실제 값 | `c, ok := a.Content.Get()`에서 `ok`가 true |

- `Content`, `Sentiment`, `Companies`, `Events`, `EventTag.CompanyIDs`는 `finlight.Field[T]`입니다.
  `ContentPresent()`, `SentimentPresent()`, `CompaniesPresent()`, `EventsPresent()`를 사용하세요.
- 빈 `Companies`나 `Events` 목록은 태깅했지만 찾은 것이 없다는 뜻입니다. 없음은 결코 `[]`로 읽히지 않으며,
  키가 없으면 `a.Companies.Value()`는 nil입니다.
- `EventTag.CompanyIDs`는 같은 응답에 `Companies`가 있을 때만 존재합니다.
- `fullArticleAccess`가 없으면 `Source.HasContent`가 없습니다. "콘텐츠 없음"이 아니라 "알 수 없음"입니다.
- 타임스탬프는 전송된 문자열 그대로입니다(`PublishedAt`). `PublishedAtTime()` 등 `...Time()` 메서드는
  RFC 3339로 엄격하게 파싱하며, 읽을 수 없으면 `(time.Time{}, false)`를 반환합니다.
- 새 서버가 보내는 알 수 없는 키는 무시됩니다. `Article`, `CompanyRecord`, `Source`, `StreamEvent`,
  `WebhookEvent`, `APIError`의 `RawJSON()`은 받은 JSON을 제공합니다. `json.Marshal`은 모델을 다시 쓸 때
  없는 키는 생략하고 null은 유지합니다.
- 어휘 값은 이름 있는 문자열 타입(`finlight.EventType`, `finlight.CompanyRole` 등)입니다. 상수
  (`finlight.EventTypeMNA`, `finlight.CompanyRoleAcquirer` 등)와 비교하세요. 알 수 없는 값은 그대로
  유지됩니다. `finlight.EventTypeValues()` 등이 알려진 값을 나열합니다.

## 실시간 스트림

```go
includeUpdates := true
s, err := client.Stream(ctx, finlight.StreamParams{
	Query:          "ticker:NVDA",
	Language:       "en",
	Include:        []finlight.IncludeBlock{finlight.IncludeBlockCompanies, finlight.IncludeBlockEvents},
	IncludeUpdates: &includeUpdates,
}, &finlight.StreamOptions{
	OnReconnect: func(r finlight.ReconnectInfo) { log.Println("reconnected", r.Reason, r.LastSeenAt) },
})
if err != nil {
	log.Fatal(err) // a parameter was rejected locally
}
for ev, err := range s.Events() {
	if err != nil {
		log.Fatal(err) // terminal, see below
	}
	if ev.Kind == finlight.StreamEventKindOther {
		continue // an event this SDK does not know yet; ev.RawJSON() has the frame
	}
	fmt.Println(ev.Kind, ev.Data.Title)
}
```

- `ev.Kind`는 `created`, `updated`, `other` 중 하나입니다.
- `client.RawStream(ctx, finlight.RawStreamParams{...}, opts)`는 보강 전의 기사를 전달합니다. include가 없고
  content, sentiment, companies, events도 없습니다.
- `StreamOptions`의 훅: `OnAdmitted(AdmittedInfo)`, 첫 번째를 제외한 새 연결마다 `OnReconnect(ReconnectInfo)`,
  스트림을 끝내지 않는 문제에 대한 `OnError(finlight.Error)`, `OnQuotaWait(QuotaWaitInfo)`. 훅은 전용
  goroutine에서 순서대로 하나씩 실행되므로 느린 훅이 ping과 pong을 늦추지 않습니다. 루프와 공유하는 상태에는
  잠금이 필요합니다. 훅 안에서 `s.Close()`를 호출해도 됩니다. 훅 안의 panic은 복구되어 로그에 남으며 스트림을 멈추지 않습니다.
- 스트림은 25초마다 ping을 보내고, 60초 동안 pong이 없으면 재연결하며, 115분 후 새 연결을 엽니다. 재연결할
  때마다 같은 subscribe를 보내고 중복을 버립니다. 재연결 전 대기 시간은 연결이 기사나 pong을 전달할 때까지
  늘어납니다(1초에서 30초).
- 자동으로 누락분을 채우지 않습니다. `ReconnectInfo.LastSeenAt`은 마지막으로 본 `availableAt`
  (raw: `firstSeenAt`)입니다. `Articles.SearchAll`에 `From: *info.LastSeenAt, OrderBy: finlight.OrderByAvailableAt`
  (raw: `OrderByFirstSeenAt`)을 지정해 빈 구간을 채우세요.
- 치명적인 문제는 마지막 `(StreamEvent{}, err)`로 루프를 끝냅니다: 잘못된 키(`*APIError`, `APIKind`가
  `unauthorized`), WebSocket 권한 없음(`forbidden`), subscribe 거부(`validation_failed`), 다른 연결이
  넘겨받은 경우의 `*finlight.PreemptedError`. 그 밖의 문제는 재연결하고 `OnError`로 전달됩니다.
- 월간 WebSocket 쿼터를 다 쓴 경우(연결 거부, 또는 사유 `quota_exceeded`의 preempt) 스트림은 끝나지 않고
  쿼터가 허용될 때까지 1시간마다 다시 시도합니다. 대기할 때마다 경고를 로그에 남기고 `OnQuotaWait`를
  호출합니다. `Config.DisableQuotaReconnect`를 설정하면 오류로 스트림을 끝냅니다.
  `QuotaReconnectInterval`로 간격을 바꿀 수 있습니다.
- `break`, context 취소, `s.Close()`로 멈춥니다. 소켓은 1000으로 닫히고 모든 goroutine과 타이머가 끝납니다.
  context를 취소하면 루프는 `ctx.Err()`로 끝납니다.
- 파라미터는 `Stream`을 호출할 때 검사되며, 연결은 `Events()`를 순회할 때 열립니다.

알아 둘 제한:

- 요금제의 연결 수 제한은 enriched, raw, 이전 SDK 연결이 함께 씁니다. 제한에 도달하면 새 연결은 거부되고
  SDK는 60초 후 다시 시도합니다. `StreamOptions.Takeover`를 설정하면 가장 오래된 연결을 대체합니다.
- subscribe 직후 서버는 조건에 맞는 최신 기사를 한 번 보냅니다(재연결 후에도). 오래된 기사일 수 있습니다.
- 32,000 bytes를 넘는 실시간 프레임은 서버에서 알림 없이 버려집니다. 긴 기사에 `IncludeBlockContent`를
  쓰면 생길 수 있습니다. `Articles.Get`으로 가져오세요.
- `connection.admitted`는 서버가 subscribe를 검사하기 전에 도착합니다. 수락되었다는 뜻이 아닙니다.

## Webhook

finlight는 모든 Webhook에 서명합니다. 무엇이든 파싱하기 전에 **원본** 요청 본문으로 검증하세요.

```go
import "github.com/callbk/finlight-client-go/v2/webhook"

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body) // the raw bytes, exactly as received
	ev, err := webhook.ConstructEvent(body, r.Header, secret)
	var wrong *webhook.WrongPayloadVersionError
	switch {
	case errors.As(err, &wrong):
		keepForLater(wrong.Raw) // an old v2 body: answer 2xx anyway
		w.WriteHeader(http.StatusNoContent)
		return
	case err != nil:
		http.Error(w, "invalid webhook", http.StatusBadRequest) // never 410
		return
	}
	if alreadySeen(ev.EventID) { // deliveries can repeat: deduplicate on EventID
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusNoContent) // answer within 5 s, work afterwards
	go process(ev)
}
```

- `header`는 `http.Header`이며 이름은 대소문자를 구분하지 않고 찾습니다.
  `webhook.ConstructEventFromValues(body, signature, timestamp, secret)`는 두 헤더 값을 직접 받고,
  `webhook.VerifySignature`는 검사만 합니다.
- 타임스탬프는 앞뒤 모두 `webhook.DefaultTolerance`(300초) 이내여야 합니다
  (`webhook.WithTolerance`, `webhook.WithNow`).
- `*webhook.VerificationError`는 `Reason`(예: `signature_mismatch`, `timestamp_outside_tolerance`)을 가집니다.
- `*webhook.WrongPayloadVersionError`는 검증 실패가 아닙니다. 서명은 유효하지만 본문이 `payloadVersion` 3이
  아닙니다. `DetectedVersion`, `PayloadVersion`, `Raw`, `Link`를 가집니다.
- 대시보드 테스트 Webhook에서는 `ev.IsTest`가 true입니다. 요금제 제한을 받지 않습니다.
- 410으로는 절대 응답하지 마세요(never 410). Webhook이 즉시 비활성화됩니다. 2xx가 아닌 응답이 10번 연속되어도
  비활성화됩니다.

원본 본문 얻기:

| 프레임워크 | 원본 본문 |
|---|---|
| `net/http`, chi, gorilla/mux | `io.ReadAll(r.Body)` |
| Gin | `c.GetRawData()` |
| Echo | `io.ReadAll(c.Request().Body)` |
| Fiber | `c.Body()`(handler가 반환되기 전에 복사) |

## 오류와 재시도

SDK의 모든 오류는 `Kind()`가 있는 `finlight.Error`를 구현합니다: `api`, `validation`, `network`, `timeout`,
`decode`, `webhook_verification`, `preempted`. 타입은 `errors.As`로 얻으세요.

| 오류 | 발생 시점 |
|---|---|
| `*finlight.APIError` | API가 오류로 응답: REST, 거부된 스트림 연결, 스트림 오류. `Status`, `Code`, `Type`, `Title`, `Detail`, `RequestID`, `Errors`(`FieldIssue`), `APIKind`, `Raw`. `APIKind`는 `validation_failed`, `cursor_invalid`, `cursor_mismatch`, `unauthorized`, `forbidden`, `not_found`, `rate_limited`, `internal_error`, `other`(상수 `finlight.APIKind...`). |
| `rate_limited`인 `*finlight.APIError` | `LimitType`은 `monthly`, `burst`, `unknown` 또는 새로운 값. `RetryAfter`는 헤더 값. |
| `*finlight.ValidationError` | 요청이 로컬에서 거부됨. `Param`이 키를 알려 줍니다. |
| `*finlight.NetworkError`, `*finlight.TimeoutError` | 네트워크 장애 또는 타임아웃(타임아웃은 `*NetworkError`와도 일치). `Unwrap`으로 원래 오류를 얻습니다. |
| `*finlight.DecodeError` | 응답을 읽을 수 없음(`Path`, `Raw`). |
| `*webhook.VerificationError` | Webhook 검증 실패(`Reason`). |
| `*finlight.PreemptedError` | 다른 연결이 스트림을 넘겨받음(`Reason`, `NewLeaseID`). `DisableQuotaReconnect`일 때는 쿼터 소진 시에도 발생. |

재시도: 5xx 응답, 네트워크 오류, 시도별 타임아웃은 검색을 포함해 대기 시간을 늘리며 최대 3번 재시도합니다.
400, 401, 403, 404와 월간 429는 재시도하지 않습니다. 월간 429는 `SearchAll` 안에서도 즉시 실패하며, 쿼터를
기다리는 것은 스트림뿐입니다(위 참고). 버스트 429는 정확히 `Retry-After`초(헤더가 없으면 10초)를 기다린 뒤 한 번
더 시도합니다. 기본으로 켜져 있고 `MaxRetries`에 포함되지 않으며 `RetryOnBurst: finlight.BurstRetryOff`로 끌 수
있습니다. 호출마다 고유한 `X-Request-Id`를 보내고 재시도에서도 같은 값을 씁니다. context를 취소하면 대기 중에도
즉시 멈추며, 오류는 `ctx.Err()`이고 재시도하지 않습니다. SDK 아래에 숨은 재시도는 없습니다. net/http가 GET을
스스로 다시 보내지 않고, 리디렉션도 따르지 않습니다(3xx는 `*APIError`로 돌아옵니다).

## 설정

```go
client, err := finlight.NewClient(finlight.Config{
	APIKey:  os.Getenv("FINLIGHT_API_KEY"),
	BaseURL: "https://api.finlight.me", // a trailing slash is fine
	WSSURL:  "wss://wss.finlight.me",   // raw streams use <WSSURL>/raw
	Timeout: 30 * time.Second,          // per attempt
})
```

0 값은 기본값을 뜻합니다. 시간은 `time.Duration`입니다. 기본으로 켜진 기능은 음수(`MaxRetries`,
`CompanyCacheSize`)나 `Disable...` 필드로 끌 수 있습니다.

| 키 | 기본값 | 의미 |
|---|---|---|
| `APIKey` | 필수 | API 키, 비어 있으면 안 됨 |
| `BaseURL` | `https://api.finlight.me` | REST 기본 URL |
| `WSSURL` | `wss://wss.finlight.me` | WebSocket URL, raw는 `/raw` |
| `Timeout` | 30초 | REST 시도 1회(SDK 자체 HTTP 클라이언트에만) |
| `TotalTimeout` | 없음 | 호출당 상한(재시도와 버스트 대기 포함) |
| `MaxRetries` | 3 | 5xx, 네트워크 오류, 타임아웃 재시도 횟수(음수: 없음) |
| `RetryBaseDelay`, `RetryMaxDelay` | 0.5초, 8초 | 재시도 사이에 늘어나는 대기 |
| `RetryOnBurst` | `once` | `finlight.BurstRetryOff`로 버스트 재시도 끄기 |
| `BurstWait` | 10초 | `Retry-After`가 없을 때의 버스트 대기 |
| `MaxPages` | 10 | `SearchAll`의 페이지 상한 |
| `MaxResponseBytes` | 32 MiB | REST 응답 크기 상한 |
| `CompanyCacheSize` | 256 | `Companies.Get`이 보관하는 레코드 수(음수: 끔) |
| `PingInterval` | 25초 | 스트림 ping |
| `WatchdogTimeout` | 60초 | 이 시간 동안 pong이 없으면 재연결 |
| `RotationInterval` | 115분 | 2시간 제한 전에 새 연결 |
| `ReconnectBaseDelay`, `ReconnectMaxDelay` | 1초, 30초 | 재연결 사이에 늘어나는 대기 |
| `LongBackoff` | 60초 | 연결 수 제한이나 429로 거부된 뒤의 대기 |
| `DisableQuotaReconnect` | false | 월간 WebSocket 쿼터를 기다리지 않고 스트림 종료 |
| `QuotaReconnectInterval` | 1시간 | 쿼터 재시도 간격 |
| `SearchFailedThreshold` | 5 | `OnError` 전까지 연속 `SEARCH_FAILED` 횟수 |
| `DedupeCacheSize` | 1000 | 스트림별로 기억하는 전달 수 |
| `MaxMessageBytes` | 32 MiB | 스트림 메시지 크기 상한(최소 16 MiB) |
| `ConnectTimeout` | 30초 | WebSocket 핸드셰이크 |
| `Logger` | 출력 없음 | `*slog.Logger` |
| `Clock` | 시스템 시계 | 대기와 스트림 타이머의 시간 원천(테스트용) |
| `HTTPClient` | 자체 클라이언트 | REST와 WebSocket 핸드셰이크에 쓰는 `*http.Client` |

직접 만든 `HTTPClient`를 쓰면 그 타임아웃, 프록시, transport가 적용됩니다(`Timeout`은 추가되지 않음).
시도마다 쿼터가 들기 때문에 재시도하는 transport는 추가하지 마세요. SDK는 그것을 닫지 않습니다. 키가 다른
호스트로 가지 않도록 SDK는 리디렉션을 끈 복사본을 씁니다.

## 로깅

SDK는 `log/slog`로 `Config.Logger`에 `logger=finlight` 속성과 함께 기록합니다. logger가 없으면 아무것도
출력하지 않습니다. 경고는 건너뛴 항목, 재시도, 재연결, 쿼터 대기를 다룹니다. API 키, Webhook 시크릿, 서명,
전체 본문은 로그, 오류 메시지, `Config`와 `Client`의 `%v` 출력에 나타나지 않습니다. SDK는 stdout이나 stderr에
쓰지 않고, 기본 logger를 바꾸지 않으며, 시그널 핸들러도 설치하지 않습니다.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := finlight.NewClient(finlight.Config{APIKey: key, Logger: logger})
```

## 데이터 참고 사항

- 오래된 기사는 `PublishedAt`이 `"1970-01-01T00:00:00.000Z"`일 수 있습니다. 받은 그대로 전달됩니다.
- 빈 텍스트는 null입니다(`Summary`는 nil, `Content.IsNull()`).
- 역할 도입 전에 태깅된 기사는 `Role`과 `Relevance`가 nil이고 `Events`가 `[]`입니다.
- ID 도입 전에 색인된 기사에는 `Articles.Get(id)`가 404를 반환합니다. `GetByLink`를 쓰세요.
- `AvailableAt`은 오래된 기사에서 nil이며 raw 스트림에서는 항상 nil입니다.

## v1에서 마이그레이션

전체 가이드는 [docs.finlight.me](https://docs.finlight.me/migration/v2-to-v3)에 있습니다. 요약:

| v1 | v2 |
|---|---|
| `import "github.com/callbk/finlight-client-go"` | `import finlight "github.com/callbk/finlight-client-go/v2"` |
| `finlight.New(finlight.Config{...})` | `finlight.NewClient(finlight.Config{...})` |
| `client.Articles.FetchArticles(ctx, GetArticlesParams{...})` | `client.Articles.Search(ctx, SearchParams{...})`, `client.Articles.SearchAll(...)` |
| `Page` / `PageSize` | `Limit` / `Cursor` |
| `IncludeContent`, `IncludeEntities` | `Include: []finlight.IncludeBlock{...}`(`content`, `companies`, `events`) |
| `Order: "DESC"` | `Order: finlight.SortOrderDesc` |
| `PublishDate`, `CreatedAt`, `RevisedDate` | `PublishedAt`, `FirstSeenAt`, `RevisedAt` |
| `Images` | `Media[].URL` |
| `Source`(문자열) | `Source.Domain` |
| `Sentiment`, `Confidence` | `Sentiment.Value().Label`, `Sentiment.Value().Confidence` |
| `client.Websocket.Stream(ctx, ...)` | `client.Stream(ctx, ...)`와 `range s.Events()` |
| raw WebSocket 클라이언트 | `client.RawStream(ctx, ...)` |
| Webhook `IsUpdate` | `ev.Event == finlight.WebhookEventNameArticleUpdated` |
| Webhook 본문 | `ev.Data`, 그리고 중복 제거용 `ev.EventID` |

Webhook을 v3로 전환하는 네 단계:

1. 이 SDK를 배포합니다.
2. `*webhook.WrongPayloadVersionError`에 2xx로 응답하고 그 `Raw` JSON을 보관합니다.
3. 대시보드에서 Webhook을 `payloadVersion` 3으로 전환합니다.
4. 약 2시간 기다립니다. 전환 전에 대기열에 들어간 재전송은 여전히 이전 형식으로 도착합니다.

같은 키에서 v1과 v2 스트림을 나란히 실행하지 마세요. 연결 수 제한을 공유합니다.

## 개발

```bash
go test -race ./...                                              # unit and conformance tests, no network
go run ./internal/tools/synckit <monorepo>/shared/contracts/api-v3 # copy the contract kit
go run ./internal/tools/synckit -check                           # verify it against its MANIFEST
```

`testdata/conformance/`는 계약 키트의 복사본이며 `CONTRACT_VERSION`과 sha256 `MANIFEST`를 포함합니다.
[CASES.md](CASES.md)는 각 키트 케이스와 테스트의 대응표입니다. 라이브 테스트는 dev에서만 실행하며
`FINLIGHT_API_KEY`, `FINLIGHT_BASE_URL`, `FINLIGHT_WSS_URL`이 필요합니다: `go test -tags integration -run Live .`

## 라이선스

MIT
