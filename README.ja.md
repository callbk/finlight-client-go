# finlight client for Go

*[English](README.md) | [简体中文](README.zh-CN.md) | 日本語 | [한국어](README.ko.md)*

> この文書は [README.md](README.md) の翻訳で、更新が遅れることがあります。内容が異なる場合は英語版が優先されます。

> **Breaking release.** モジュール `github.com/callbk/finlight-client-go/v2`（v2.0.0）は finlight API v3 のみに対応します。
> v1 向けのコードはそのままではコンパイルできません。v1（API v2）を使い続ける場合は
> `github.com/callbk/finlight-client-go`（`/v2` なし）をインポートしてください。[v1 からの移行](#v1-からの移行) を参照してください。

[finlight](https://finlight.me) ニュース API の Go クライアントです。REST で金融ニュース、企業、ソースを
検索・参照し、WebSocket で記事をリアルタイムに受信し、Webhook を検証できます。

## バージョン

| SDK version | API version | Status |
|---|---|---|
| v2.x (`github.com/callbk/finlight-client-go/v2`) | v3（REST `/v3`、WebSocket `payloadVersion` 3、v3 Webhook） | 現行 |
| v1.x (`github.com/callbk/finlight-client-go`) | v2 | 今後のリリースなし |

SDK のバージョンと API のバージョンは独立しています。API v3 に対応する finlight SDK：TypeScript 4.x、
Python 3.x、PHP 2.x、Go モジュール `/v2`、.NET 2.x、Java 1.x、Ruby 1.x、Rust 0.2。

## 動作要件

- Go 1.24 以降
- `github.com/coder/websocket` v1.8.13 以降（唯一の依存）

## インストール

```bash
go get github.com/callbk/finlight-client-go/v2@v2
```

## クイックスタート

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

すべての呼び出しは最初の引数に `context.Context` を取ります。パラメータ構造体の各フィールドは API の
キー 1 つに対応し、設定したフィールドだけが送信されます。空文字列、nil スライス、nil ポインタは
「未設定」を意味します。

## 記事を検索する

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

`From` と `To` は指定したまま送信されます。ミリ秒付きの UTC か日付（`2026-11-01`）を指定してください。
`SourceTypes` はソースの種別で記事を絞り込み、`ExcludeSourceTypes` は除外します
（`finlight.SourceTypeValues()`）。未分類ソースの記事は `SourceTypes` に一致しません。
`SearchAll` はページ送りを自動で行います。呼び出した時点でリクエスト本文が固定され、以降のページでは
cursor だけが追加されます。各ページがクォータを消費するため、`MaxPages` ページ（既定 10）で停止します。

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

続きを取得するには、同じ条件で `Cursor: *res.NextCursor()` を渡します。結果は 1 回しか走査できず、
2 回目は `*finlight.ValidationError` になります。形の誤り（例：`Limit: 101`、`Languages: []string{}`）は
送信前に `*finlight.ValidationError` で失敗します。キーの集合は構造体の型で固定されています。語彙の値は
ローカルでは検査しないため、サーバー側の新しい値は SDK を更新せずに使えます。

## 記事・企業・ソースを参照する

```go
article, err := client.Articles.Get(ctx, "EU8JwpsZ8iJehuyPZjMqRz",
	&finlight.GetOptions{Include: []finlight.IncludeBlock{finlight.IncludeBlockContent}})
article, err = client.Articles.GetByLink(ctx, "https://www.reuters.com/...", nil)

companies, err := client.Companies.Search(ctx, finlight.CompanySearchParams{Q: "NVDA", MIC: "XNAS"}) // at most 10, no pages
companies, err = client.Companies.GetMany(ctx, []int64{4821, 1001})
company, err := client.Companies.Get(ctx, 4821, nil) // the SDK keeps no cache
fmt.Println(company.CacheInfo().ETag, company.CacheInfo().CacheControl)

sources, err := client.Sources.List(ctx)
```

- 見つからない場合は `APIKind` が `not_found` の `*finlight.APIError` になり、空の結果にはなりません。
- 記事 ID 導入前に索引された記事はリンクでのみ取得できます。
- 企業の参照には `entityAccess` 権限が必要です（ない場合は `APIKind` が `forbidden`）。
- 企業・ソースの参照は記事リクエストと同じく、リクエストごとに 1 クレジットを消費します（`Companies.GetMany` は 1 リクエストで 1）。
  304 の再検証とエラー応答も 1 クレジットを消費します。参照でも 429 が返ることがあります。
- ソースが未分類の場合、`Source.Type` と `Article.Source.Type` は nil です。
- SDK は企業レコードをキャッシュしません。`CacheInfo()` は `ETag` と `CacheControl` を返すので、
  レコードはご自身で保持できます。クレジットを節約するには、`max-age` の間は再度呼び出さないでください。
  保持しているレコードを確認するには、その ETag を `&finlight.CompanyGetOptions{IfNoneMatch: etag}` で渡します。
  304 応答は `(nil, nil)` を返し、これも 1 クレジットを消費します。`IfNoneMatch` がなければ `If-None-Match` ヘッダーは送られません。
- `Companies.Search`、`Companies.GetMany`、`Sources.List` は `Items` と `Skipped` を返します。
  検索ページの `ArticlePage.Skipped` も同様で、読み取れなかったエントリがここに入ります。

## 欠落・null・空

一部のキーはプランや `include` によって変わり、最大 3 つの状態があります。

| 状態 | 意味 | 確認方法 |
|---|---|---|
| 欠落 | 未リクエスト、権限なし、または raw ストリーム | `a.ContentPresent()` が false |
| null | リクエスト済みで権限もあるが値がない | `a.ContentPresent()` が true かつ `a.Content.IsNull()` |
| 値 | 値そのもの | `c, ok := a.Content.Get()` で `ok` が true |

- `Content`、`Sentiment`、`Companies`、`Events`、`EventTag.CompanyIDs` は `finlight.Field[T]` です。
  `ContentPresent()`、`SentimentPresent()`、`CompaniesPresent()`、`EventsPresent()` を使ってください。
- 空の `Companies` や `Events` は、タグ付けされたが何も見つからなかったことを意味します。欠落が `[]` として
  読まれることはありません。キーが欠落していると `a.Companies.Value()` は nil です。
- `EventTag.CompanyIDs` は同じ応答に `Companies` がある場合にのみ存在します。
- `fullArticleAccess` がない場合 `Source.HasContent` は欠落します。「コンテンツなし」ではなく「不明」です。
- タイムスタンプは送信された文字列のままです（`PublishedAt`）。`PublishedAtTime()` などの `...Time()` は
  RFC 3339 として厳密に解析し、読めない場合は `(time.Time{}, false)` を返します。
- 新しいサーバーからの未知のキーは無視されます。`Article`、`CompanyRecord`、`Source`、`StreamEvent`、
  `WebhookEvent`、`APIError` の `RawJSON()` は受信した JSON を返します。`json.Marshal` はモデルを書き戻す際、
  欠落したキーを省き、null を保持します。
- 語彙の値は名前付き文字列型（`finlight.EventType`、`finlight.CompanyRole` など）です。定数
  （`finlight.EventTypeMNA`、`finlight.CompanyRoleAcquirer` など）と比較してください。未知の値はそのまま
  保持されます。`finlight.EventTypeValues()` などが既知の値を列挙します。

## リアルタイムストリーム

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

- `ev.Kind` は `created`、`updated`、`other` のいずれかです。
- `client.RawStream(ctx, finlight.RawStreamParams{...}, opts)` はエンリッチ前の記事を配信します。
  include はなく、content、sentiment、companies、events もありません。
- `SourceTypes` と `ExcludeSourceTypes` は検索と同じく、両方のストリームをソースの種別で絞り込みます。未知の値は
  致命的な `*APIError`（`APIKind` は `validation_failed`、`AllowedValues` あり）でストリームを終了させます。
- `StreamOptions` のフック：`OnAdmitted(AdmittedInfo)`、最初以外の新しい接続ごとの
  `OnReconnect(ReconnectInfo)`、ストリームを終了させない問題の `OnError(finlight.Error)`、
  `OnQuotaWait(QuotaWaitInfo)`。フックは専用の goroutine で順番に 1 つずつ実行されるため、遅いフックが
  ping や pong を遅らせることはありません。ループと共有する状態にはロックが必要です。フックから `s.Close()` を
  呼べます。フック内の panic は回復されてログに記録され、ストリームは止まりません。
- ストリームは 25 秒ごとに ping を送り、60 秒間 pong がなければ再接続し、115 分後に新しい接続を開きます。
  再接続のたびに同じ subscribe を送り、重複を破棄します。再接続までの待ち時間は、接続が記事か pong を
  届けるまで伸びていきます（1 秒から 30 秒）。
- 自動での取り戻しは行いません。`ReconnectInfo.LastSeenAt` は最後に見た `availableAt`（raw：`firstSeenAt`）
  です。欠落分は `Articles.SearchAll` に `From: *info.LastSeenAt, OrderBy: finlight.OrderByAvailableAt`
  （raw：`OrderByFirstSeenAt`）を指定して取得してください。
- 致命的な問題は最後の `(StreamEvent{}, err)` でループを終えます：無効なキー（`*APIError`、`APIKind` は
  `unauthorized`）、WebSocket 権限なし（`forbidden`）、subscribe の拒否（`validation_failed`）、別の接続が
  引き継いだ場合の `*finlight.PreemptedError`。その他の問題は再接続し、`OnError` に渡されます。
- 月間 WebSocket クォータを使い切った場合（接続拒否、または理由 `quota_exceeded` の preempt）、
  ストリームは終了せず、クォータが戻るまで 1 時間ごとに再試行します。待機のたびに警告をログに出し
  `OnQuotaWait` を呼びます。`Config.DisableQuotaReconnect` を設定するとエラーで終了します。
  `QuotaReconnectInterval` で間隔を変更できます。
- `break`、context のキャンセル、`s.Close()` で停止します。ソケットは 1000 で閉じられ、すべての goroutine と
  タイマーが終了します。context をキャンセルするとループは `ctx.Err()` で終わります。
- パラメータは `Stream` 呼び出し時に検査され、接続は `Events()` を走査したときに開かれます。

知っておくべき制限：

- プランの接続数上限は enriched、raw、旧 SDK の接続で共有されます。上限に達すると新しい接続は拒否され、
  SDK は 60 秒後に再試行します。`StreamOptions.Takeover` を設定すると最も古い接続を置き換えます。
- subscribe の直後に、条件に合う最新の記事が 1 件送られます（再接続のたびにも）。古い記事のこともあります。
- 32,000 bytes を超えるライブフレームはサーバーで通知なく破棄されます。長い記事で `IncludeBlockContent` を
  使うと起こり得ます。`Articles.Get` で取得してください。
- `connection.admitted` はサーバーが subscribe を検査する前に届きます。受け入れられたことを意味しません。

## Webhook

finlight はすべての Webhook に署名します。何かを解析する前に、**生の**リクエスト本文で検証してください。

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

- `header` は `http.Header` で、名前は大文字小文字を区別せずに照合されます。
  `webhook.ConstructEventFromValues(body, signature, timestamp, secret)` は 2 つのヘッダー値を直接受け取り、
  `webhook.VerifySignature` は検査だけを行います。
- タイムスタンプは前後とも `webhook.DefaultTolerance`（300 秒）以内である必要があります
  （`webhook.WithTolerance`、`webhook.WithNow`）。
- `*webhook.VerificationError` は `Reason`（例：`signature_mismatch`、`timestamp_outside_tolerance`）を持ちます。
- `*webhook.WrongPayloadVersionError` は検証失敗ではありません。署名は有効ですが本文が `payloadVersion` 3 では
  ありません。`DetectedVersion`、`PayloadVersion`、`Raw`、`Link` を持ちます。
- ダッシュボードのテスト Webhook では `ev.IsTest` が true です。プランによる制限を受けません。
- 410 は絶対に返さないでください（never 410）。Webhook が即座に無効化されます。2xx 以外が 10 回続いても
  無効化されます。

生の本文の取得方法：

| フレームワーク | 生の本文 |
|---|---|
| `net/http`、chi、gorilla/mux | `io.ReadAll(r.Body)` |
| Gin | `c.GetRawData()` |
| Echo | `io.ReadAll(c.Request().Body)` |
| Fiber | `c.Body()`（handler が戻る前にコピー） |

## エラーとリトライ

SDK のすべてのエラーは `Kind()` を持つ `finlight.Error` を実装します：`api`、`validation`、`network`、
`timeout`、`decode`、`webhook_verification`、`preempted`。型は `errors.As` で取得してください。

| エラー | 発生条件 |
|---|---|
| `*finlight.APIError` | API がエラーを返した：REST、拒否されたストリーム接続、ストリームのエラー。`Status`、`Code`、`Type`、`Title`、`Detail`、`RequestID`、`Errors`（`FieldIssue`）、`APIKind`、`Raw`。`APIKind` は `validation_failed`、`cursor_invalid`、`cursor_mismatch`、`unauthorized`、`forbidden`、`not_found`、`rate_limited`、`internal_error`、`other`（定数 `finlight.APIKind...`）。 |
| `rate_limited` の `*finlight.APIError` | `LimitType` は `monthly`、`burst`、`unknown` または新しい値。`RetryAfter` はヘッダーの値。 |
| `*finlight.ValidationError` | リクエストがローカルで拒否された。`Param` がキーを示します。 |
| `*finlight.NetworkError`、`*finlight.TimeoutError` | ネットワーク障害またはタイムアウト（タイムアウトは `*NetworkError` にも一致）。`Unwrap` で元のエラーを取得。 |
| `*finlight.DecodeError` | 応答を読み取れなかった（`Path`、`Raw`）。 |
| `*webhook.VerificationError` | Webhook の検証に失敗した（`Reason`）。 |
| `*finlight.PreemptedError` | 別の接続がストリームを引き継いだ（`Reason`、`NewLeaseID`）。`DisableQuotaReconnect` 時はクォータ切れでも発生。 |

リトライ：5xx 応答、ネットワークエラー、試行ごとのタイムアウトは、検索も含め、待機を延ばしながら最大 3 回
再試行します。400、401、403、404、月間 429 は再試行しません。月間 429 は `SearchAll` 内でも即座に失敗し、
クォータを待つのはストリームだけです（上記参照）。バースト 429 は `Retry-After` 秒（ヘッダーがなければ 10 秒）
ちょうど待ってから 1 回だけ再試行します。既定で有効で、`MaxRetries` には数えず、
`RetryOnBurst: finlight.BurstRetryOff` で無効にできます。呼び出しごとに独自の `X-Request-Id` を送り、
再試行でも同じ値を使います。context をキャンセルすると待機中も含めて即座に停止し、エラーは `ctx.Err()` で、
再試行されません。SDK の下に隠れた再試行はありません。net/http が GET を勝手に再送することはなく、
リダイレクトにも従いません（3xx は `*APIError` として返ります）。

## 設定

```go
client, err := finlight.NewClient(finlight.Config{
	APIKey:  os.Getenv("FINLIGHT_API_KEY"),
	BaseURL: "https://api.finlight.me", // a trailing slash is fine
	WSSURL:  "wss://wss.finlight.me",   // raw streams use <WSSURL>/raw
	Timeout: 30 * time.Second,          // per attempt
})
```

ゼロ値は既定値を意味します。時間は `time.Duration` です。既定で有効な機能は負の数（`MaxRetries`）
または `Disable...` フィールドで無効にできます。

| キー | 既定値 | 意味 |
|---|---|---|
| `APIKey` | 必須 | API キー、空は不可 |
| `BaseURL` | `https://api.finlight.me` | REST のベース URL |
| `WSSURL` | `wss://wss.finlight.me` | WebSocket URL、raw は `/raw` |
| `Timeout` | 30 秒 | REST の 1 回の試行（SDK 自身の HTTP クライアントのみ） |
| `TotalTimeout` | なし | 1 回の呼び出しの上限（再試行とバースト待機を含む） |
| `MaxRetries` | 3 | 5xx、ネットワークエラー、タイムアウトの再試行回数（負：なし） |
| `RetryBaseDelay`、`RetryMaxDelay` | 0.5 秒、8 秒 | 再試行間の増加する待機 |
| `RetryOnBurst` | `once` | `finlight.BurstRetryOff` でバースト再試行を無効化 |
| `BurstWait` | 10 秒 | `Retry-After` がない場合のバースト待機 |
| `MaxPages` | 10 | `SearchAll` のページ上限 |
| `MaxResponseBytes` | 32 MiB | REST 応答のサイズ上限 |
| `PingInterval` | 25 秒 | ストリームの ping |
| `WatchdogTimeout` | 60 秒 | この時間 pong がなければ再接続 |
| `RotationInterval` | 115 分 | 2 時間の上限前に新しい接続 |
| `ReconnectBaseDelay`、`ReconnectMaxDelay` | 1 秒、30 秒 | 再接続間の増加する待機 |
| `LongBackoff` | 60 秒 | 接続数上限や 429 で拒否された後の待機 |
| `DisableQuotaReconnect` | false | 月間 WebSocket クォータを待たずにストリームを終了 |
| `QuotaReconnectInterval` | 1 時間 | クォータ再試行の間隔 |
| `SearchFailedThreshold` | 5 | `OnError` までの連続 `SEARCH_FAILED` 回数 |
| `DedupeCacheSize` | 1000 | ストリームごとに記憶する配信数 |
| `MaxMessageBytes` | 32 MiB | ストリームメッセージのサイズ上限（最低 16 MiB） |
| `ConnectTimeout` | 30 秒 | WebSocket ハンドシェイク |
| `Logger` | 出力なし | `*slog.Logger` |
| `Clock` | システム時計 | 待機とストリームタイマーの時刻源（テスト用） |
| `HTTPClient` | 独自のクライアント | REST と WebSocket ハンドシェイクに使う `*http.Client` |

独自の `HTTPClient` を使う場合、そのタイムアウト、プロキシ、transport が適用されます（`Timeout` は追加
されません）。各試行がクォータを消費するため、再試行する transport は追加しないでください。SDK がそれを
閉じることはありません。キーが別のホストに送られないよう、SDK はリダイレクトを無効にしたコピーを使います。

## ログ

SDK は `log/slog` で `Config.Logger` に属性 `logger=finlight` 付きで出力します。logger がなければ何も出力しません。
警告はスキップしたエントリ、再試行、再接続、クォータ待機を対象とします。API キー、Webhook シークレット、署名、
本文全体がログ、エラーメッセージ、`Config` と `Client` の `%v` 出力に現れることはありません。SDK は stdout や
stderr に書き込まず、既定の logger を変更せず、シグナルハンドラーも登録しません。

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := finlight.NewClient(finlight.Config{APIKey: key, Logger: logger})
```

## データに関する注意

- 古い記事は `PublishedAt` が `"1970-01-01T00:00:00.000Z"` の場合があります。そのまま配信されます。
- 空のテキストは null です（`Summary` は nil、`Content.IsNull()`）。
- ロール導入前にタグ付けされた記事は `Role` と `Relevance` が nil、`Events` が `[]` です。
- ID 導入前に索引された記事では `Articles.Get(id)` が 404 を返します。`GetByLink` を使ってください。
- `AvailableAt` は古い記事では nil、raw ストリームでは常に nil です。

## v1 からの移行

完全なガイドは [docs.finlight.me](https://docs.finlight.me/migration/v2-to-v3) にあります。概要：

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
| `Source`（文字列） | `Source.Domain` |
| `Sentiment`、`Confidence` | `Sentiment.Value().Label`、`Sentiment.Value().Confidence` |
| `client.Websocket.Stream(ctx, ...)` | `client.Stream(ctx, ...)` と `range s.Events()` |
| raw WebSocket クライアント | `client.RawStream(ctx, ...)` |
| Webhook の `IsUpdate` | `ev.Event == finlight.WebhookEventNameArticleUpdated` |
| Webhook の本文 | `ev.Data` と、重複排除用の `ev.EventID` |

Webhook を v3 に切り替える 4 つの手順：

1. この SDK をデプロイします。
2. `*webhook.WrongPayloadVersionError` に 2xx で応答し、その `Raw` JSON を保持します。
3. ダッシュボードで Webhook を `payloadVersion` 3 に切り替えます。
4. 約 2 時間待ちます。切り替え前にキューに入った再送は旧形式のまま届きます。

同じキーで v1 と v2 のストリームを並行して動かさないでください。接続数上限を共有します。

## 開発

```bash
go test -race ./...                                              # unit and conformance tests, no network
go run ./internal/tools/synckit <monorepo>/shared/contracts/api-v3 # copy the contract kit
go run ./internal/tools/synckit -check                           # verify it against its MANIFEST
```

`testdata/conformance/` は契約キットのコピーで、`CONTRACT_VERSION` と sha256 の `MANIFEST` を含みます。
[CASES.md](CASES.md) は各ケースとテストの対応表です。ライブテストは dev に対してのみ実行し、
`FINLIGHT_API_KEY`、`FINLIGHT_BASE_URL`、`FINLIGHT_WSS_URL` が必要です：`go test -tags integration -run Live .`

## ライセンス

MIT
