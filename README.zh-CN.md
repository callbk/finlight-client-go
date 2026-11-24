# finlight client for Go

*[English](README.md) | 简体中文 | [日本語](README.ja.md) | [한국어](README.ko.md)*

> 本文是 [README.md](README.md) 的译文，可能会滞后更新。如有出入，以英文版为准。

> **Breaking release.** 模块 `github.com/callbk/finlight-client-go/v2`（v2.0.0）只支持 finlight API v3。
> 为 v1 编写的代码无法直接编译。如需继续使用 v1（API v2），请继续导入
> `github.com/callbk/finlight-client-go`（不带 `/v2`）。参见 [从 v1 迁移](#从-v1-迁移)。

[finlight](https://finlight.me) 新闻 API 的 Go 客户端：通过 REST 搜索和查询金融新闻、公司和来源，
通过 WebSocket 实时接收文章，并验证 Webhook。

## 版本

| SDK version | API version | Status |
|---|---|---|
| v2.x (`github.com/callbk/finlight-client-go/v2`) | v3（REST `/v3`、WebSocket `payloadVersion` 3、v3 Webhook） | 当前版本 |
| v1.x (`github.com/callbk/finlight-client-go`) | v2 | 不再发布新版本 |

SDK 版本与 API 版本相互独立。支持 API v3 的 finlight SDK：TypeScript 4.x、Python 3.x、PHP 2.x、
Go 模块 `/v2`、.NET 2.x、Java 1.x、Ruby 1.x、Rust 0.2。

## 环境要求

- Go 1.24 或更高版本
- `github.com/coder/websocket` v1.8.13 或更高版本（唯一的依赖）

## 安装

```bash
go get github.com/callbk/finlight-client-go/v2@v2
```

## 快速开始

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

每个调用的第一个参数都是 `context.Context`。参数结构体的每个字段对应 API 的一个键，只有设置过的字段才会
发送：空字符串、nil 切片和 nil 指针表示“未设置”。

## 搜索文章

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

`From` 和 `To` 按原样发送：请使用带毫秒的 UTC 时间或日期（`2026-11-01`）。
`SourceTypes` 只保留、`ExcludeSourceTypes` 排除指定来源类型的文章（`finlight.SourceTypeValues()`）；
未分类来源的文章永远不匹配 `SourceTypes`。
`SearchAll` 会自动翻页。调用时请求体即被冻结，后续页面只添加 cursor。由于每一页都消耗配额，
它在 `MaxPages` 页（默认 10）之后停止。

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

如需继续，请使用相同的过滤条件并传入 `Cursor: *res.NextCursor()`。结果只能遍历一次；第二次遍历会得到
`*finlight.ValidationError`。形状错误（例如 `Limit: 101` 或 `Languages: []string{}`）会在发送前以
`*finlight.ValidationError` 失败。键的集合由结构体类型固定；词汇表的值不在本地检查，因此服务器的新值
无需更新 SDK 即可使用。

## 查询文章、公司和来源

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

- 未找到时返回 `APIKind` 为 `not_found` 的 `*finlight.APIError`，绝不会返回空结果。
- 在文章 ID 引入之前索引的文章只能通过链接获取。
- 公司查询需要 `entityAccess` 权限（否则 `APIKind` 为 `forbidden`）。
- 每次公司或来源查询与文章请求一样消耗 1 个额度（`Companies.GetMany` 每个请求计 1 次）。
  304 重新验证和错误响应也消耗 1 个额度；查询也可能返回 429。
- 来源未分类时，`Source.Type` 和 `Article.Source.Type` 为 nil。
- `Companies.Get` 会保存最近的记录，并用其 ETag 重新验证（`CompanyCacheSize`）。
  使用自己的 `&finlight.CompanyGetOptions{IfNoneMatch: etag}` 时，304 响应返回 `(nil, nil)`。
  `CacheInfo()` 提供 `ETag`、`CacheControl` 和 `NotModified`。
- `Companies.Search`、`Companies.GetMany` 和 `Sources.List` 返回 `Items` 和 `Skipped`；
  搜索页的 `ArticlePage.Skipped` 同理。无法读取的条目会放在这里。

## 缺失、null 与空

部分键取决于您的套餐或 `include`，最多有三种状态：

| 状态 | 含义 | 检查方式 |
|---|---|---|
| 缺失 | 未请求、无权限或 raw 流 | `a.ContentPresent()` 为 false |
| null | 已请求且有权限，但没有值 | `a.ContentPresent()` 为 true 且 `a.Content.IsNull()` |
| 值 | 实际的值 | `c, ok := a.Content.Get()` 且 `ok` 为 true |

- `Content`、`Sentiment`、`Companies`、`Events` 和 `EventTag.CompanyIDs` 是 `finlight.Field[T]`。
  请使用 `ContentPresent()`、`SentimentPresent()`、`CompaniesPresent()` 和 `EventsPresent()`。
- 空的 `Companies` 或 `Events` 列表表示文章已被标注但没有结果。缺失绝不会读作 `[]`：键缺失时
  `a.Companies.Value()` 为 nil。
- 只有同一响应中包含 `Companies` 时才会有 `EventTag.CompanyIDs`。
- 没有 `fullArticleAccess` 时 `Source.HasContent` 缺失：表示未知，而不是“没有内容”。
- 时间戳保持发送时的字符串（`PublishedAt`）。`PublishedAtTime()` 等 `...Time()` 方法严格按 RFC 3339
  解析，无法读取时返回 `(time.Time{}, false)`。
- 新服务器发送的未知键会被忽略。`Article`、`CompanyRecord`、`Source`、`StreamEvent`、`WebhookEvent`
  和 `APIError` 的 `RawJSON()` 返回收到的 JSON。`json.Marshal` 写回模型时保留 null，省略缺失的键。
- 词汇表的值是具名字符串类型（`finlight.EventType`、`finlight.CompanyRole` 等）。请与常量比较
  （`finlight.EventTypeMNA`、`finlight.CompanyRoleAcquirer` 等）；未知值按原样保留。
  `finlight.EventTypeValues()` 等函数列出已知的值。

## 实时流

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

- `ev.Kind` 为 `created`、`updated` 或 `other`。
- `client.RawStream(ctx, finlight.RawStreamParams{...}, opts)` 提供丰富处理之前的文章：没有 include，
  没有 content、sentiment、companies 或 events。
- `SourceTypes` 和 `ExcludeSourceTypes` 与搜索中一样，按来源类型过滤两种流。未知的值会以致命的
  `*APIError` 结束流（`APIKind` 为 `validation_failed`，并带有 `AllowedValues`）。
- `StreamOptions` 中的钩子：`OnAdmitted(AdmittedInfo)`、除第一次外每次新连接后的
  `OnReconnect(ReconnectInfo)`、不会结束流的问题 `OnError(finlight.Error)`，以及
  `OnQuotaWait(QuotaWaitInfo)`。钩子在自己的 goroutine 上按顺序逐个运行：慢的钩子不会延误 ping 和 pong。
  与循环共享的状态需要加锁。钩子中可以调用 `s.Close()`。钩子中的 panic 会被恢复并记录，绝不会停止流。
- 流每 25 秒发送一次 ping，60 秒内没有 pong 时重新连接，并在 115 分钟后建立新连接。每次重连后发送相同的
  subscribe，并丢弃重复消息。重连前的等待会逐步增加（1 秒到 30 秒），直到某个连接送达文章或 pong。
- 不会自动补数据。`ReconnectInfo.LastSeenAt` 是您看到的最新 `availableAt`（raw：`firstSeenAt`）。
  请用 `Articles.SearchAll` 并设置 `From: *info.LastSeenAt, OrderBy: finlight.OrderByAvailableAt`
  （raw：`OrderByFirstSeenAt`）补齐缺口。
- 致命问题以最后一个 `(StreamEvent{}, err)` 结束循环：无效密钥（`*APIError`，`APIKind` 为
  `unauthorized`）、没有 WebSocket 权限（`forbidden`）、subscribe 被拒绝（`validation_failed`），
  以及其他连接接管时的 `*finlight.PreemptedError`。其他问题会重连并交给 `OnError`。
- 每月 WebSocket 配额用尽时（连接被拒绝，或收到原因为 `quota_exceeded` 的 preempt），流不会结束：
  每小时重试一次，直到配额恢复。每次等待都会记录警告并调用 `OnQuotaWait`。设置
  `Config.DisableQuotaReconnect` 可改为以错误结束流；`QuotaReconnectInterval` 可修改间隔。
- 通过 `break`、取消 context 或 `s.Close()` 停止。socket 以 1000 关闭，所有 goroutine 和计时器都会结束。
  取消 context 时循环以 `ctx.Err()` 结束。
- 参数在调用 `Stream` 时检查；遍历 `Events()` 时才建立连接。

需要了解的限制：

- 套餐的连接数上限由 enriched、raw 和旧版 SDK 的连接共享。达到上限时新连接会被拒绝，SDK 会在 60 秒后
  重试。设置 `StreamOptions.Takeover` 可改为替换最旧的连接。
- 订阅后服务器会立即发送一次最新的匹配文章（每次重连后也会）。它可能是旧文章。
- 超过 32,000 bytes 的实时帧会被服务器直接丢弃，不做通知。在长文章上使用 `IncludeBlockContent` 时
  可能发生。请用 `Articles.Get` 获取。
- `connection.admitted` 在服务器检查 subscribe 之前到达，并不表示 subscribe 已被接受。

## Webhook

finlight 会为每个 Webhook 签名。请在解析之前用**原始**请求体进行验证：

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

- `header` 是 `http.Header`；名称匹配不区分大小写。
  `webhook.ConstructEventFromValues(body, signature, timestamp, secret)` 直接接收两个头的值；
  `webhook.VerifySignature` 只做检查。
- 时间戳必须在 `webhook.DefaultTolerance`（300 秒）以内，前后两个方向都适用
  （`webhook.WithTolerance`、`webhook.WithNow`）。
- `*webhook.VerificationError` 带有 `Reason`，例如 `signature_mismatch` 或 `timestamp_outside_tolerance`。
- `*webhook.WrongPayloadVersionError` 不是验证失败：签名有效，但请求体不是 `payloadVersion` 3。
  它带有 `DetectedVersion`、`PayloadVersion`、`Raw` 和 `Link`。
- 控制台测试 Webhook 的 `ev.IsTest` 为 true，它不受套餐限制。
- 永远不要返回 410（never 410）：它会立即停用 Webhook。连续 10 次非 2xx 响应也会停用它。

获取原始请求体：

| 框架 | 原始请求体 |
|---|---|
| `net/http`、chi、gorilla/mux | `io.ReadAll(r.Body)` |
| Gin | `c.GetRawData()` |
| Echo | `io.ReadAll(c.Request().Body)` |
| Fiber | `c.Body()`（在 handler 返回前复制） |

## 错误与重试

SDK 的每个错误都实现带 `Kind()` 的 `finlight.Error`：`api`、`validation`、`network`、`timeout`、
`decode`、`webhook_verification` 或 `preempted`。请用 `errors.As` 获取具体类型。

| 错误 | 时机 |
|---|---|
| `*finlight.APIError` | API 返回错误：REST、被拒绝的流连接或流错误。`Status`、`Code`、`Type`、`Title`、`Detail`、`RequestID`、`Errors`（`FieldIssue`）、`APIKind`、`Raw`。`APIKind` 为 `validation_failed`、`cursor_invalid`、`cursor_mismatch`、`unauthorized`、`forbidden`、`not_found`、`rate_limited`、`internal_error` 或 `other`（常量 `finlight.APIKind...`）。 |
| `rate_limited` 的 `*finlight.APIError` | `LimitType` 为 `monthly`、`burst`、`unknown` 或更新的值；`RetryAfter` 为响应头。 |
| `*finlight.ValidationError` | 请求在本地被拒绝；`Param` 指出键名。 |
| `*finlight.NetworkError`、`*finlight.TimeoutError` | 网络故障或超时（超时也匹配 `*NetworkError`）。`Unwrap` 返回原始错误。 |
| `*finlight.DecodeError` | 无法读取响应（`Path`、`Raw`）。 |
| `*webhook.VerificationError` | Webhook 验证失败（`Reason`）。 |
| `*finlight.PreemptedError` | 其他连接接管了流（`Reason`、`NewLeaseID`）。设置 `DisableQuotaReconnect` 时配额用尽也会出现。 |

重试：5xx 响应、网络错误和单次请求超时最多重试 3 次，等待时间逐步增加，搜索也包括在内。400、401、403、
404 和每月 429 绝不重试：每月 429 会立即失败，`SearchAll` 内也是如此；只有流会等待配额（见上文）。
突发 429 会精确等待 `Retry-After` 秒（无该头时为 10 秒）并再试一次；默认开启，不计入 `MaxRetries`，
`RetryOnBurst: finlight.BurstRetryOff` 可将其关闭。每个调用发送自己的 `X-Request-Id`，重试时保持不变。
取消 context 会立即停止调用（包括等待）；错误为 `ctx.Err()`，绝不重试。SDK 之下没有隐藏的重试：net/http
不会自行重发 GET，也不会跟随重定向（3xx 以 `*APIError` 返回）。

## 配置

```go
client, err := finlight.NewClient(finlight.Config{
	APIKey:  os.Getenv("FINLIGHT_API_KEY"),
	BaseURL: "https://api.finlight.me", // a trailing slash is fine
	WSSURL:  "wss://wss.finlight.me",   // raw streams use <WSSURL>/raw
	Timeout: 30 * time.Second,          // per attempt
})
```

零值表示默认值。时长使用 `time.Duration`。默认开启的功能可用负数（`MaxRetries`、`CompanyCacheSize`）
或 `Disable...` 字段关闭。

| 键 | 默认值 | 含义 |
|---|---|---|
| `APIKey` | 必填 | API 密钥，不能为空 |
| `BaseURL` | `https://api.finlight.me` | REST 基础 URL |
| `WSSURL` | `wss://wss.finlight.me` | WebSocket URL，raw 在 `/raw` |
| `Timeout` | 30 秒 | 每次 REST 尝试（仅限 SDK 自己的 HTTP 客户端） |
| `TotalTimeout` | 无 | 每次调用的上限，包括重试和突发等待 |
| `MaxRetries` | 3 | 5xx、网络错误和超时的重试次数（负数：不重试） |
| `RetryBaseDelay`、`RetryMaxDelay` | 0.5 秒、8 秒 | 重试之间逐步增加的等待 |
| `RetryOnBurst` | `once` | `finlight.BurstRetryOff` 关闭突发重试 |
| `BurstWait` | 10 秒 | 缺少 `Retry-After` 时的突发等待 |
| `MaxPages` | 10 | `SearchAll` 的页数上限 |
| `MaxResponseBytes` | 32 MiB | REST 响应的大小上限 |
| `CompanyCacheSize` | 256 | `Companies.Get` 保存的记录数（负数：关闭） |
| `PingInterval` | 25 秒 | 流的 ping |
| `WatchdogTimeout` | 60 秒 | 超过该时间没有 pong 时重连 |
| `RotationInterval` | 115 分钟 | 在 2 小时上限前建立新连接 |
| `ReconnectBaseDelay`、`ReconnectMaxDelay` | 1 秒、30 秒 | 重连之间逐步增加的等待 |
| `LongBackoff` | 60 秒 | 因连接数上限或 429 被拒绝后的等待 |
| `DisableQuotaReconnect` | false | 不等待每月 WebSocket 配额，直接结束流 |
| `QuotaReconnectInterval` | 1 小时 | 配额重试之间的等待 |
| `SearchFailedThreshold` | 5 | 连续多少次 `SEARCH_FAILED` 后调用 `OnError` |
| `DedupeCacheSize` | 1000 | 每个流记住的投递数 |
| `MaxMessageBytes` | 32 MiB | 流消息的大小上限（至少 16 MiB） |
| `ConnectTimeout` | 30 秒 | WebSocket 握手 |
| `Logger` | 静默 | `*slog.Logger` |
| `Clock` | 系统时钟 | 等待和流计时器的时间源（用于测试） |
| `HTTPClient` | 自带客户端 | 您的 `*http.Client`，用于 REST 和 WebSocket 握手 |

使用自己的 `HTTPClient` 时，适用其超时、代理和 transport（不会再加 `Timeout`）；请不要添加带重试的
transport，因为每次尝试都消耗配额。SDK 绝不会关闭它。SDK 使用它的一个副本并关闭重定向，以免密钥被发往其他主机。

## 日志

SDK 通过 `log/slog` 写入 `Config.Logger`，带有属性 `logger=finlight`。未设置 logger 时不输出任何内容。
警告涵盖被跳过的条目、重试、重连和配额等待。API 密钥、Webhook 密钥、签名和完整的请求体绝不会出现在日志、
错误信息或 `Config` 与 `Client` 的 `%v` 输出中。SDK 从不写入 stdout 或 stderr，不修改默认 logger，
也不安装信号处理器。

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := finlight.NewClient(finlight.Config{APIKey: key, Logger: logger})
```

## 数据说明

- 旧文章可能带有 `PublishedAt` `"1970-01-01T00:00:00.000Z"`，按原样提供。
- 空文本为 null（`Summary` 为 nil，`Content.IsNull()`）。
- 在引入角色之前标注的文章，`Role` 和 `Relevance` 为 nil，`Events` 为 `[]`。
- 对于在 ID 引入之前索引的文章，`Articles.Get(id)` 返回 404。请使用 `GetByLink`。
- 旧文章的 `AvailableAt` 为 nil，raw 流中始终为 nil。

## 从 v1 迁移

完整指南见 [docs.finlight.me](https://docs.finlight.me/migration/v2-to-v3)。简要对照：

| v1 | v2 |
|---|---|
| `import "github.com/callbk/finlight-client-go"` | `import finlight "github.com/callbk/finlight-client-go/v2"` |
| `finlight.New(finlight.Config{...})` | `finlight.NewClient(finlight.Config{...})` |
| `client.Articles.FetchArticles(ctx, GetArticlesParams{...})` | `client.Articles.Search(ctx, SearchParams{...})`、`client.Articles.SearchAll(...)` |
| `Page` / `PageSize` | `Limit` / `Cursor` |
| `IncludeContent`、`IncludeEntities` | `Include: []finlight.IncludeBlock{...}`（`content`、`companies`、`events`） |
| `Order: "DESC"` | `Order: finlight.SortOrderDesc` |
| `PublishDate`、`CreatedAt`、`RevisedDate` | `PublishedAt`、`FirstSeenAt`、`RevisedAt` |
| `Images` | `Media[].URL` |
| `Source`（字符串） | `Source.Domain` |
| `Sentiment`、`Confidence` | `Sentiment.Value().Label`、`Sentiment.Value().Confidence` |
| `client.Websocket.Stream(ctx, ...)` | `client.Stream(ctx, ...)` 与 `range s.Events()` |
| raw WebSocket 客户端 | `client.RawStream(ctx, ...)` |
| Webhook `IsUpdate` | `ev.Event == finlight.WebhookEventNameArticleUpdated` |
| Webhook 请求体 | `ev.Data`，以及用于去重的 `ev.EventID` |

将 Webhook 切换到 v3 的四个步骤：

1. 部署本 SDK。
2. 对 `*webhook.WrongPayloadVersionError` 返回 2xx，并保存其 `Raw` JSON。
3. 在控制台将 Webhook 切换到 `payloadVersion` 3。
4. 等待约 2 小时：切换前排队的重试仍会以旧格式到达。

不要在同一个密钥上同时运行 v1 和 v2 的流：它们共享连接数上限。

## 开发

```bash
go test -race ./...                                              # unit and conformance tests, no network
go run ./internal/tools/synckit <monorepo>/shared/contracts/api-v3 # copy the contract kit
go run ./internal/tools/synckit -check                           # verify it against its MANIFEST
```

`testdata/conformance/` 是契约套件的副本，包含 `CONTRACT_VERSION` 和 sha256 `MANIFEST`；
[CASES.md](CASES.md) 将每个用例映射到其测试。实时测试只针对 dev 运行，需要 `FINLIGHT_API_KEY`、
`FINLIGHT_BASE_URL` 和 `FINLIGHT_WSS_URL`：`go test -tags integration -run Live .`

## 许可证

MIT
