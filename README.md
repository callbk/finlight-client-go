# finlight client for Go

*English | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [한국어](README.ko.md)*

> **Breaking release.** Module `github.com/callbk/finlight-client-go/v2` (v2.0.0) speaks only the finlight
> API v3. Code written for v1 does not compile unchanged. To stay on v1 (API v2), keep importing
> `github.com/callbk/finlight-client-go` (without `/v2`). See [Migrating from v1](#migrating-from-v1).

Go client for the [finlight](https://finlight.me) news API: search and look up financial news,
companies and sources over REST, stream articles in real time over WebSocket, and verify webhooks.

## Versions

| SDK version | API version | Status |
|---|---|---|
| v2.x (`github.com/callbk/finlight-client-go/v2`) | v3 (REST `/v3`, WebSocket `payloadVersion` 3, v3 webhooks) | current |
| v1.x (`github.com/callbk/finlight-client-go`) | v2 | no further releases |

SDK and API versions are independent. The finlight SDKs that speak API v3: TypeScript 4.x, Python 3.x,
PHP 2.x, Go module `/v2`, .NET 2.x, Java 1.x, Ruby 1.x, Rust 0.2.

## Requirements

- Go 1.24 or newer
- `github.com/coder/websocket` v1.8.13 or newer (the only dependency)

## Installation

```bash
go get github.com/callbk/finlight-client-go/v2@v2
```

## Quick start

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

Every call takes a `context.Context` first. Parameter fields map to exactly one key of the API each, and
only fields you set are sent: an empty string, a nil slice and a nil pointer mean "not set".

## Search articles

```go
rel := 0.5
page, err := client.Articles.Search(ctx, finlight.SearchParams{
	Query:        "eventType:mna AND companyCountry:US",
	Languages:    []string{"en", "de"},
	From:         time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05.000Z"),
	EventTypes:   []finlight.EventType{finlight.EventTypeMNA},
	SourceTypes:  []finlight.SourceType{finlight.SourceTypeNewswire, finlight.SourceTypeFinancialMedia},
	MinRelevance: &rel,
	OrderBy:      finlight.OrderByAvailableAt,
	Include:      []finlight.IncludeBlock{finlight.IncludeBlockContent, finlight.IncludeBlockCompanies, finlight.IncludeBlockEvents},
	Limit:        50,
})
fmt.Println(page.Pagination.HasMore, page.Pagination.NextCursor)
```

`From` and `To` are sent as you give them: send UTC with milliseconds or a date (`2026-11-01`).
`SourceTypes` keeps and `ExcludeSourceTypes` drops articles by the type of their source
(`finlight.SourceTypeValues()`); articles of unclassified sources never match `SourceTypes`.
`SearchAll` walks the pages for you. It freezes the request body when you call it and only adds the
cursor on later pages. It stops after `MaxPages` pages (default 10), because every page costs quota.

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

Pass `Cursor: *res.NextCursor()` with the same filters to continue. A result can be iterated once; a
second iteration yields a `*finlight.ValidationError`. Wrong shapes (for example `Limit: 101` or
`Languages: []string{}`) fail with `*finlight.ValidationError` before anything is sent. The set of keys
is fixed by the struct types; values of vocabularies are not checked locally, so new server values work
without an SDK update.

## Look up articles, companies and sources

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

- A miss is an `*finlight.APIError` with `APIKind` `not_found`; it is never an empty result.
- Articles indexed before article ids existed can only be fetched by link.
- Company lookups need the `entityAccess` entitlement (`APIKind` `forbidden` otherwise).
- Each successful company or source lookup costs 1 credit (`Companies.GetMany` one per request).
  A 304 revalidation and error answers are free; a lookup can also get a 429.
- `Source.Type` and `Article.Source.Type` are nil when the source is not classified.
- `Companies.Get` keeps the last records and revalidates them with their ETag (`CompanyCacheSize`).
  With your own `&finlight.CompanyGetOptions{IfNoneMatch: etag}`, a 304 answer returns `(nil, nil)`.
  `CacheInfo()` gives `ETag`, `CacheControl` and `NotModified`.
- `Companies.Search`, `Companies.GetMany` and `Sources.List` return `Items` and `Skipped`;
  `ArticlePage.Skipped` does the same for search pages. Entries that cannot be read land there.

## Absent, null and empty

Some keys depend on your plan or on `include`. They have up to three states:

| State | Meaning | How to check |
|---|---|---|
| absent | not requested, not entitled, or a raw stream | `a.ContentPresent()` is false |
| null | requested and entitled, but no value | `a.ContentPresent()` is true and `a.Content.IsNull()` |
| value | the value | `c, ok := a.Content.Get()` with `ok` true |

- `Content`, `Sentiment`, `Companies`, `Events` and `EventTag.CompanyIDs` are `finlight.Field[T]` values.
  Use `ContentPresent()`, `SentimentPresent()`, `CompaniesPresent()` and `EventsPresent()`.
- An empty `Companies` or `Events` list means the article was tagged and nothing was found. Absent never
  reads as `[]`: `a.Companies.Value()` is nil when the key was absent.
- `EventTag.CompanyIDs` is absent unless `Companies` is in the same answer.
- `Source.HasContent` is absent without `fullArticleAccess`: unknown, not "no content".
- Timestamps stay the strings that were sent (`PublishedAt`). `PublishedAtTime()` and the other
  `...Time()` methods parse them strictly as RFC 3339 and return `(time.Time{}, false)` for anything
  unreadable.
- Unknown keys from newer servers are ignored. `RawJSON()` on `Article`, `CompanyRecord`, `Source`,
  `StreamEvent`, `WebhookEvent` and `APIError` gives the JSON as received. `json.Marshal` writes a model
  back with absent keys left out and null kept.
- Vocabulary values are named string types (`finlight.EventType`, `finlight.CompanyRole`, ...). Compare
  them with the constants (`finlight.EventTypeMNA`, `finlight.CompanyRoleAcquirer`, ...); unknown values
  are kept as sent. `finlight.EventTypeValues()` and its siblings list the known values.

## Real-time streams

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

- `ev.Kind` is `created`, `updated` or `other`.
- `client.RawStream(ctx, finlight.RawStreamParams{...}, opts)` delivers articles before enrichment: no
  include, no content, sentiment, companies or events.
- `SourceTypes` and `ExcludeSourceTypes` filter both streams by source type, as in a search. An unknown
  value ends the stream with a terminal `*APIError` (`APIKind` `validation_failed`, `AllowedValues` set).
- Hooks in `StreamOptions`: `OnAdmitted(AdmittedInfo)`, `OnReconnect(ReconnectInfo)` after every new
  connection but the first, `OnError(finlight.Error)` for problems that do not end the stream, and
  `OnQuotaWait(QuotaWaitInfo)`. Hooks run one at a time, in order, on their own goroutine: a slow hook
  never delays pings or pongs. State shared with the loop needs a lock. A hook may call `s.Close()`. A panic in a hook is recovered and logged and
  never stops the stream.
- The stream pings every 25 s, reconnects when no pong came for 60 s, and opens a fresh connection
  after 115 minutes. After each reconnect it sends the same subscribe and drops duplicates. The wait
  before a reconnect grows (1 s up to 30 s) until a connection delivers an article or a pong.
- It never backfills. `ReconnectInfo.LastSeenAt` is the latest `availableAt` (raw: `firstSeenAt`) you saw.
  Fill gaps with `Articles.SearchAll` with `From: *info.LastSeenAt, OrderBy: finlight.OrderByAvailableAt`
  (raw: `OrderByFirstSeenAt`).
- Terminal problems end the loop with a last `(StreamEvent{}, err)`: an invalid key (`*APIError`,
  `APIKind` `unauthorized`), no WebSocket access (`forbidden`), a rejected subscribe
  (`validation_failed`), and `*finlight.PreemptedError` when another connection took over. Other problems
  reconnect and go to `OnError`.
- When the monthly WebSocket quota is used up (the dial is refused, or a preempt with reason
  `quota_exceeded`), the stream does not end: it tries again every hour until the quota allows it. Each
  wait logs a warning and calls `OnQuotaWait`. Set `Config.DisableQuotaReconnect` to end the stream with
  the error instead; `QuotaReconnectInterval` changes the hour.
- Stop with `break`, by cancelling the context, or with `s.Close()`. The socket is closed with 1000 and
  every goroutine and timer ends. A cancelled context ends the loop with `ctx.Err()`.
- Parameters are checked when you call `Stream`; the connection opens when you range over `Events()`.

Limits to know:

- Your plan's connection limit is shared by enriched, raw and older SDK connections. A new connection
  while the limit is reached is refused; the SDK retries after 60 s. Set `StreamOptions.Takeover` to
  replace the oldest connection instead.
- Right after subscribing, the server sends the newest matching article once (also after every
  reconnect). It can be old.
- Live frames over 32,000 bytes are dropped by the server without notice. This can happen with
  `IncludeBlockContent` on long articles. Fetch them with `Articles.Get`.
- `connection.admitted` arrives before the server checks the subscribe. It does not mean it was accepted.

## Webhooks

finlight signs every webhook. Verify it with the **raw** request body, before parsing anything:

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

- `header` is an `http.Header`; names are matched without regard to case.
  `webhook.ConstructEventFromValues(body, signature, timestamp, secret)` takes the two header values
  directly; `webhook.VerifySignature` only checks.
- The timestamp must be within `webhook.DefaultTolerance` (300 s) in both directions
  (`webhook.WithTolerance`, `webhook.WithNow`).
- `*webhook.VerificationError` carries `Reason`, for example `signature_mismatch` or
  `timestamp_outside_tolerance`.
- `*webhook.WrongPayloadVersionError` is not a verification failure: the signature was valid, but the
  body is not `payloadVersion` 3. It carries `DetectedVersion`, `PayloadVersion`, `Raw` and `Link`.
- `ev.IsTest` is true for the dashboard test webhook. It is not cut by your plan.
- Never answer with 410: it disables the webhook at once. Ten non-2xx answers in a row disable it too.

Getting the raw body:

| Framework | Raw body |
|---|---|
| `net/http`, chi, gorilla/mux | `io.ReadAll(r.Body)` |
| Gin | `c.GetRawData()` |
| Echo | `io.ReadAll(c.Request().Body)` |
| Fiber | `c.Body()` (copy it before the handler returns) |

## Errors and retries

Every SDK error implements `finlight.Error` with `Kind()`: `api`, `validation`, `network`, `timeout`,
`decode`, `webhook_verification` or `preempted`. Use `errors.As` to get the type.

| Error | When |
|---|---|
| `*finlight.APIError` | The API answered with an error: REST, a refused stream connection, or a stream error. `Status`, `Code`, `Type`, `Title`, `Detail`, `RequestID`, `Errors` (`FieldIssue`), `APIKind`, `Raw`. `APIKind` is `validation_failed`, `cursor_invalid`, `cursor_mismatch`, `unauthorized`, `forbidden`, `not_found`, `rate_limited`, `internal_error` or `other` (constants `finlight.APIKind...`). |
| `*finlight.APIError` with `rate_limited` | `LimitType` is `monthly`, `burst`, `unknown` or a newer value; `RetryAfter` is the header. |
| `*finlight.ValidationError` | A request was rejected locally; `Param` names the key. |
| `*finlight.NetworkError`, `*finlight.TimeoutError` | Network failure or timeout (a timeout also matches `*NetworkError`). `Unwrap` gives the original error. |
| `*finlight.DecodeError` | A response could not be read (`Path`, `Raw`). |
| `*webhook.VerificationError` | A webhook failed verification (`Reason`). |
| `*finlight.PreemptedError` | Another connection took over the stream (`Reason`, `NewLeaseID`). With `DisableQuotaReconnect` also when the quota ran out. |

Retries: 5xx answers, network errors and attempt timeouts are retried up to 3 times with growing waits,
the search included. 400, 401, 403, 404 and a monthly 429 are never retried: a monthly 429 fails at once,
also inside `SearchAll`; only streams wait for the quota (see above). A burst 429 waits exactly
`Retry-After` seconds (10 without the header) and tries once more; this is on by default, does not count
against `MaxRetries`, and `RetryOnBurst: finlight.BurstRetryOff` turns it off. Every call sends its own
`X-Request-Id`, the same on each retry. Cancelling the context stops a call at once, waits included; the
error is `ctx.Err()`, never retried. There are no hidden retries below the SDK: net/http does not resend a
GET on its own, and redirects are not followed (a 3xx comes back as `*APIError`).

## Configuration

```go
client, err := finlight.NewClient(finlight.Config{
	APIKey:  os.Getenv("FINLIGHT_API_KEY"),
	BaseURL: "https://api.finlight.me", // a trailing slash is fine
	WSSURL:  "wss://wss.finlight.me",   // raw streams use <WSSURL>/raw
	Timeout: 30 * time.Second,          // per attempt
})
```

Zero values mean the default. Durations are `time.Duration`. Where a feature that is on by default can
be turned off, use a negative number (`MaxRetries`, `CompanyCacheSize`) or the `Disable...` field.

| Key | Default | Meaning |
|---|---|---|
| `APIKey` | required | your API key, never empty |
| `BaseURL` | `https://api.finlight.me` | REST base URL |
| `WSSURL` | `wss://wss.finlight.me` | WebSocket URL, raw at `/raw` |
| `Timeout` | 30 s | per REST attempt (only with the SDK's own HTTP client) |
| `TotalTimeout` | none | cap per call, retries and burst wait included |
| `MaxRetries` | 3 | retries for 5xx, network errors and timeouts (negative: none) |
| `RetryBaseDelay`, `RetryMaxDelay` | 0.5 s, 8 s | growing wait between retries |
| `RetryOnBurst` | `once` | `finlight.BurstRetryOff` turns the burst retry off |
| `BurstWait` | 10 s | burst wait when `Retry-After` is missing |
| `MaxPages` | 10 | page cap of `SearchAll` |
| `MaxResponseBytes` | 32 MiB | size limit of a REST answer |
| `CompanyCacheSize` | 256 | records kept by `Companies.Get` (negative: off) |
| `PingInterval` | 25 s | stream ping |
| `WatchdogTimeout` | 60 s | reconnect when no pong came for this long |
| `RotationInterval` | 115 min | fresh connection before the 2 h limit |
| `ReconnectBaseDelay`, `ReconnectMaxDelay` | 1 s, 30 s | growing wait between reconnects |
| `LongBackoff` | 60 s | wait after a refusal for the connection limit or a 429 |
| `DisableQuotaReconnect` | false | end the stream instead of waiting for the monthly WebSocket quota |
| `QuotaReconnectInterval` | 1 h | wait between quota attempts |
| `SearchFailedThreshold` | 5 | `SEARCH_FAILED` in a row before `OnError` |
| `DedupeCacheSize` | 1000 | remembered deliveries per stream |
| `MaxMessageBytes` | 32 MiB | size limit of a stream message (at least 16 MiB) |
| `ConnectTimeout` | 30 s | WebSocket handshake |
| `Logger` | silent | a `*slog.Logger` |
| `Clock` | system clock | time source of waits and stream timers (for tests) |
| `HTTPClient` | own client | your `*http.Client`, used for REST and the WebSocket handshake |

With your own `HTTPClient`, its timeouts, proxy and transport apply (`Timeout` is not added); do not add
retrying transports, because every attempt costs quota. The SDK never closes it. It uses a copy of it
with redirects turned off, so that the key never goes to another host.

## Logging

The SDK logs through `log/slog` to `Config.Logger`, with the attribute `logger=finlight`. Without a
logger it is silent. Warnings cover skipped entries, retries, reconnects and quota waits. The API key, the
webhook secret, signatures and full bodies never appear in logs, error messages or `%v` output of
`Config` and `Client`. The SDK never writes to stdout or stderr, never changes the default logger and
installs no signal handlers.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := finlight.NewClient(finlight.Config{APIKey: key, Logger: logger})
```

## Data notes

- Old articles can carry `PublishedAt` `"1970-01-01T00:00:00.000Z"`. It is delivered as sent.
- Empty texts come as null (`Summary` is nil, `Content.IsNull()`).
- Articles tagged before roles existed have `Role` and `Relevance` nil and `Events` `[]`.
- `Articles.Get(id)` answers 404 for articles indexed before ids existed. Use `GetByLink`.
- `AvailableAt` is nil on older articles and always nil on raw streams.

## Migrating from v1

The full guide is on [docs.finlight.me](https://docs.finlight.me/migration/v2-to-v3). The short version:

| v1 | v2 |
|---|---|
| `import "github.com/callbk/finlight-client-go"` | `import finlight "github.com/callbk/finlight-client-go/v2"` |
| `finlight.New(finlight.Config{...})` | `finlight.NewClient(finlight.Config{...})` |
| `client.Articles.FetchArticles(ctx, GetArticlesParams{...})` | `client.Articles.Search(ctx, SearchParams{...})`, `client.Articles.SearchAll(...)` |
| `Page` / `PageSize` | `Limit` / `Cursor` |
| `IncludeContent`, `IncludeEntities` | `Include: []finlight.IncludeBlock{...}` (`content`, `companies`, `events`) |
| `Order: "DESC"` | `Order: finlight.SortOrderDesc` |
| `PublishDate`, `CreatedAt`, `RevisedDate` | `PublishedAt`, `FirstSeenAt`, `RevisedAt` |
| `Images` | `Media[].URL` |
| `Source` (string) | `Source.Domain` |
| `Sentiment`, `Confidence` | `Sentiment.Value().Label`, `Sentiment.Value().Confidence` |
| `client.Websocket.Stream(ctx, ...)` | `client.Stream(ctx, ...)` and `range s.Events()` |
| raw WebSocket client | `client.RawStream(ctx, ...)` |
| webhook `IsUpdate` | `ev.Event == finlight.WebhookEventNameArticleUpdated` |
| webhook body | `ev.Data`, plus `ev.EventID` for deduplication |

Switching a webhook to v3, in four steps:

1. Deploy this SDK.
2. Answer `*webhook.WrongPayloadVersionError` with 2xx and keep its `Raw` JSON.
3. Switch the webhook to `payloadVersion` 3 in the dashboard.
4. Wait about 2 hours: retries queued before the switch still arrive in the old format.

Do not run v1 and v2 streams side by side on one key: they share the connection limit.

## Development

```bash
go test -race ./...                                              # unit and conformance tests, no network
go run ./internal/tools/synckit <monorepo>/shared/contracts/api-v3 # copy the contract kit
go run ./internal/tools/synckit -check                           # verify it against its MANIFEST
```

`testdata/conformance/` is a copy of the contract kit with its `CONTRACT_VERSION` and sha256 `MANIFEST`;
[CASES.md](CASES.md) maps every kit case to its test. Live tests run only against dev and need
`FINLIGHT_API_KEY`, `FINLIGHT_BASE_URL` and `FINLIGHT_WSS_URL`:
`go test -tags integration -run Live .`

## License

MIT
