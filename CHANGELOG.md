# Changelog

## v2.0.0 (unreleased)

**Breaking release.** The module path is now `github.com/callbk/finlight-client-go/v2`, and v2.0.0
speaks only the finlight API v3 (REST `/v3`, WebSocket `payloadVersion` 3, v3 webhooks). Code written for
v1 does not compile unchanged. To stay on v1 (API v2), keep importing `github.com/callbk/finlight-client-go`.
Migration guide: https://docs.finlight.me/migration/v2-to-v3

Contract kit: 3.0.0-rc.3.

### Removed

- Everything from API v1 and v2: `finlight.New`, `ArticleService.FetchArticles`, `GetArticlesParams`,
  the WebSocket and raw WebSocket clients, `WebSocketOptions`, the v2 models, `FlexTime`, `FlexFloat`,
  the old webhook helpers, and the special handling of the old close codes and admin kicks.
- Go 1.23.

### Added

- `finlight.NewClient(finlight.Config)` with `Articles.Search`, `Articles.SearchAll` (`SearchAllResult`
  with `All()` and `Pages()` as `iter.Seq2`, `NextCursor`, `Truncated`, `PagesFetched`), `Articles.Get`,
  `Articles.GetByLink`, `Companies.Search`, `Companies.GetMany`, `Companies.Get` (ETag revalidation,
  `CacheInfo()`, `IfNoneMatch` with 304 as `(nil, nil)`) and `Sources.List`. Lists carry `Skipped`
  (`SkippedEntry`) for entries that could not be read.
- Tolerant models: exact key matching, required keys only where the contract says so, `RawJSON()`,
  strict `...Time()` accessors, and `Field[T]` for the gated keys (absent, null, value) with
  `ContentPresent`, `SentimentPresent`, `CompaniesPresent` and `EventsPresent`.
- Open vocabularies as named string types with constants and `...Values()` functions.
- `client.Stream` and `client.RawStream` return a `*finlight.Stream`; `Events()` is an
  `iter.Seq2[StreamEvent, error]`. Ping and pong watchdog, rotation before the 2 hour limit, the same
  subscribe after every reconnect, a duplicate filter, a `Takeover` option, typed terminal errors and the
  hooks `OnAdmitted`, `OnReconnect`, `OnError` and `OnQuotaWait`. The API key goes out as a lowercase
  `x-api-key` header.
- When the monthly WebSocket quota is used up (dial 403 "quota exceeded" or `connection.preempted` with
  reason `quota_exceeded`), the stream tries again every hour until the quota allows it, logging a warning
  and calling `OnQuotaWait` each time. On by default; `DisableQuotaReconnect` ends the stream with the
  error instead; `QuotaReconnectInterval` sets the interval. A preempt with reason `connection_limit`
  stays terminal.
- Package `webhook`: `ConstructEvent`, `ConstructEventFromValues`, `VerifySignature`, `DefaultTolerance`
  (300 s), `WithTolerance`, `WithNow`. A correctly signed body that is not `payloadVersion` 3 returns
  `*webhook.WrongPayloadVersionError` (answer it with 2xx). Failures are `*webhook.VerificationError` with
  a `Reason`.
- Errors: `finlight.Error` with `Kind()`; `*APIError` (with `APIKind`, `LimitType`, `RetryAfter`),
  `*ValidationError`, `*NetworkError`, `*TimeoutError`, `*DecodeError`, `*PreemptedError`.
- Retries: up to 3 for 5xx, network errors and attempt timeouts; never for 400, 401, 403, 404 and a
  monthly 429. A monthly 429 fails at once, also inside `SearchAll`. A burst 429 waits exactly
  `Retry-After` (10 s without it) and retries once; on by default, `RetryOnBurst: BurstRetryOff` turns it
  off. One `X-Request-Id` per call, kept on retries.
- `User-Agent: finlight-client-go/2.0.0` and `x-client-version: 2.0.0` on every request and handshake;
  `finlight.Version` is the single source.
- Conformance suite: every case of the shared API v3 contract kit runs as a Go subtest (`CASES.md`), with
  a real HTTP server and an in-process WebSocket server; the kit is checked against its MANIFEST.
- Five examples under `examples/` (search, search-all, lookup, stream, webhook-endpoint), tested in CI.

### Requirements

- Go 1.24+, `github.com/coder/websocket` v1.8.13+.
