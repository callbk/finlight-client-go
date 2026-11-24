package finlight

import (
	"context"
	"encoding/json"
	"iter"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

// TestPublicNames pins the public names of the sdk-style table (Go column).
// It fails to compile when a name is renamed or removed.
func TestPublicNames(t *testing.T) {
	var (
		_ func(Config) (*Client, error) = NewClient
		_ string                        = Version
		_ int                           = PayloadVersion

		_ func(*ArticlesService, context.Context, SearchParams) (*ArticlePage, error)                        = (*ArticlesService).Search
		_ func(*ArticlesService, context.Context, SearchParams, *SearchAllOptions) (*SearchAllResult, error) = (*ArticlesService).SearchAll
		_ func(*ArticlesService, context.Context, string, *GetOptions) (*Article, error)                     = (*ArticlesService).Get
		_ func(*ArticlesService, context.Context, string, *GetOptions) (*Article, error)                     = (*ArticlesService).GetByLink
		_ func(*CompaniesService, context.Context, CompanySearchParams) (*CompanyList, error)                = (*CompaniesService).Search
		_ func(*CompaniesService, context.Context, []int64) (*CompanyList, error)                            = (*CompaniesService).GetMany
		_ func(*CompaniesService, context.Context, int64, *CompanyGetOptions) (*CompanyRecord, error)        = (*CompaniesService).Get
		_ func(*SourcesService, context.Context) (*SourceList, error)                                        = (*SourcesService).List
		_ func(*Client, context.Context, StreamParams, *StreamOptions) (*Stream, error)                      = (*Client).Stream
		_ func(*Client, context.Context, RawStreamParams, *StreamOptions) (*Stream, error)                   = (*Client).RawStream
		_ func(*Stream) iter.Seq2[StreamEvent, error]                                                        = (*Stream).Events
		_ func(*SearchAllResult) iter.Seq2[Article, error]                                                   = (*SearchAllResult).All
		_ func(*SearchAllResult) iter.Seq2[*ArticlePage, error]                                              = (*SearchAllResult).Pages
		_ func(*Article) bool                                                                                = (*Article).ContentPresent
		_ func(*Article) bool                                                                                = (*Article).SentimentPresent
		_ func(*Article) bool                                                                                = (*Article).CompaniesPresent
		_ func(*Article) bool                                                                                = (*Article).EventsPresent
		_ func(*Article) (time.Time, bool)                                                                   = (*Article).PublishedAtTime
		_ func(*CompanyRecord) CacheInfo                                                                     = (*CompanyRecord).CacheInfo
		_ func(StreamEvent) json.RawMessage                                                                  = StreamEvent.RawJSON
		_ func(*WebhookEvent) json.RawMessage                                                                = (*WebhookEvent).RawJSON
		_ func(*APIError) json.RawMessage                                                                    = (*APIError).RawJSON
		_ func(*Stream) error                                                                                = (*Stream).Close
		_ func(*Stream) *string                                                                              = (*Stream).LastSeenAt
		_ func(*Stream) bool                                                                                 = (*Stream).Closed
		_ func(*SearchAllResult) *string                                                                     = (*SearchAllResult).NextCursor
		_ func(*SearchAllResult) bool                                                                        = (*SearchAllResult).Truncated
		_ func(*SearchAllResult) int                                                                         = (*SearchAllResult).PagesFetched
		_ Field[string]
		_ = Config{APIKey: "", BaseURL: "", WSSURL: "", Timeout: 0, TotalTimeout: 0, MaxRetries: 0,
			RetryBaseDelay: 0, RetryMaxDelay: 0, RetryOnBurst: BurstRetryOnce, BurstWait: 0, MaxPages: 0,
			MaxResponseBytes: 0, CompanyCacheSize: 0, PingInterval: 0, WatchdogTimeout: 0, RotationInterval: 0,
			ReconnectBaseDelay: 0, ReconnectMaxDelay: 0, LongBackoff: 0, DisableQuotaReconnect: false,
			QuotaReconnectInterval: 0, SearchFailedThreshold: 0, DedupeCacheSize: 0, MaxMessageBytes: 0,
			ConnectTimeout: 0, Logger: (*slog.Logger)(nil), Clock: nil, HTTPClient: (*http.Client)(nil)}
		_ = APIError{Status: 0, Code: "", Type: "", Title: "", Detail: "", RequestID: "", Errors: []FieldIssue{{Field: "", Issue: "", AllowedValues: nil, Constraints: nil}}, APIKind: APIKindOther, Raw: "", LimitType: RateLimitTypeBurst, RetryAfter: 0}
		_ = ValidationError{Param: ""}
		_ = DecodeError{Path: "", Raw: ""}
		_ = PreemptedError{Reason: PreemptReasonConnectionLimit, NewLeaseID: nil, Timestamp: 0}
		_ = TimeoutError{NetworkError: &NetworkError{}}
		_ = StreamEvent{Kind: StreamEventKindCreated, Event: StreamEventNameArticleCreated, Data: nil}
		_ = AdmittedInfo{LeaseID: "", ServerNow: 0, ClientNonce: nil}
		_ = ReconnectInfo{LastSeenAt: nil, Reason: ReconnectReasonRotation, Connection: 0}
		_ = QuotaWaitInfo{Error: nil, NextAttemptIn: 0, Attempt: 0}
		_ = StreamOptions{Takeover: false, OnAdmitted: nil, OnReconnect: nil, OnError: nil, OnQuotaWait: nil}
		_ = SkippedEntry{Index: 0, Raw: nil, Reason: ""}
		_ = CacheInfo{ETag: "", CacheControl: "", NotModified: false}
		_ = WebhookEvent{IsTest: false}
		_ = []any{ArticleSource{}, Media{}, Sentiment{}, Company{}, Listing{}, EventTag{}, Source{}, Pagination{}, ArticlePage{}, CompanyList{}, SourceList{}, SearchAllOptions{}}
		_ = []ErrorKind{ErrorKindAPI, ErrorKindValidation, ErrorKindNetwork, ErrorKindTimeout, ErrorKindDecode, ErrorKindWebhookVerification, ErrorKindPreempted}
		_ = []APIErrorKind{APIKindValidationFailed, APIKindCursorInvalid, APIKindCursorMismatch, APIKindUnauthorized, APIKindForbidden, APIKindNotFound, APIKindRateLimited, APIKindInternalError, APIKindOther}
	)
	kinds := map[ErrorKind]Error{
		ErrorKindAPI: &APIError{}, ErrorKindValidation: &ValidationError{}, ErrorKindNetwork: &NetworkError{},
		ErrorKindTimeout: &TimeoutError{NetworkError: &NetworkError{}}, ErrorKindDecode: &DecodeError{}, ErrorKindPreempted: &PreemptedError{},
	}
	for k, e := range kinds {
		if e.Kind() != k {
			t.Errorf("%T.Kind() = %s", e, e.Kind())
		}
	}
}
