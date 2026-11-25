package finlight

// Every vocabulary in this file is open: a named string type plus constants for
// the values known today. Unknown values sent by the server are kept as they
// arrive, so compare with the constants, never with a closed switch that drops
// the default case. The XxxValues functions return a fresh copy of the known
// values.

// IncludeBlock names a gated block requested through include.
type IncludeBlock string

// Known include blocks.
const (
	IncludeBlockContent   IncludeBlock = "content"
	IncludeBlockCompanies IncludeBlock = "companies"
	IncludeBlockEvents    IncludeBlock = "events"
)

// IncludeBlockValues returns the known include blocks.
func IncludeBlockValues() []IncludeBlock {
	return []IncludeBlock{IncludeBlockContent, IncludeBlockCompanies, IncludeBlockEvents}
}

// OrderBy names the timestamp a search sorts by.
type OrderBy string

// Known sort fields.
const (
	OrderByPublishedAt OrderBy = "publishedAt"
	OrderByFirstSeenAt OrderBy = "firstSeenAt"
	OrderByRevisedAt   OrderBy = "revisedAt"
	OrderByAvailableAt OrderBy = "availableAt"
)

// OrderByValues returns the known sort fields.
func OrderByValues() []OrderBy {
	return []OrderBy{OrderByPublishedAt, OrderByFirstSeenAt, OrderByRevisedAt, OrderByAvailableAt}
}

// SortOrder is the direction of a search (lower case on the wire).
type SortOrder string

// Known sort orders.
const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

// SortOrderValues returns the known sort orders.
func SortOrderValues() []SortOrder { return []SortOrder{SortOrderAsc, SortOrderDesc} }

// CompanyRole is the position of a company in a story.
type CompanyRole string

// Known company roles.
const (
	CompanyRoleAcquirer     CompanyRole = "acquirer"
	CompanyRoleTarget       CompanyRole = "target"
	CompanyRoleSeller       CompanyRole = "seller"
	CompanyRoleDefendant    CompanyRole = "defendant"
	CompanyRoleSubject      CompanyRole = "subject"
	CompanyRoleCounterparty CompanyRole = "counterparty"
	CompanyRoleSupplier     CompanyRole = "supplier"
	CompanyRoleCustomer     CompanyRole = "customer"
	CompanyRoleCompetitor   CompanyRole = "competitor"
	CompanyRoleMentioned    CompanyRole = "mentioned"
)

// CompanyRoleValues returns the known company roles.
func CompanyRoleValues() []CompanyRole {
	return []CompanyRole{
		CompanyRoleAcquirer, CompanyRoleTarget, CompanyRoleSeller, CompanyRoleDefendant,
		CompanyRoleSubject, CompanyRoleCounterparty, CompanyRoleSupplier, CompanyRoleCustomer,
		CompanyRoleCompetitor, CompanyRoleMentioned,
	}
}

// EventType is the type of a tagged event.
type EventType string

// Known event types.
const (
	EventTypeEarnings            EventType = "earnings"
	EventTypeGuidance            EventType = "guidance"
	EventTypeMNA                 EventType = "mna"
	EventTypeIPOListing          EventType = "ipo_listing"
	EventTypeCapitalAction       EventType = "capital_action"
	EventTypeRatingAction        EventType = "rating_action"
	EventTypeManagementChange    EventType = "management_change"
	EventTypeLegal               EventType = "legal"
	EventTypeRegulatory          EventType = "regulatory"
	EventTypeBankruptcyDistress  EventType = "bankruptcy_distress"
	EventTypeProductPartnership  EventType = "product_partnership"
	EventTypeInvestmentExpansion EventType = "investment_expansion"
	EventTypeOperationalIncident EventType = "operational_incident"
	EventTypeLaborRestructuring  EventType = "labor_restructuring"
	EventTypeMacro               EventType = "macro"
	EventTypeGeopolitics         EventType = "geopolitics"
)

// EventTypeValues returns the known event types.
func EventTypeValues() []EventType {
	return []EventType{
		EventTypeEarnings, EventTypeGuidance, EventTypeMNA, EventTypeIPOListing,
		EventTypeCapitalAction, EventTypeRatingAction, EventTypeManagementChange, EventTypeLegal,
		EventTypeRegulatory, EventTypeBankruptcyDistress, EventTypeProductPartnership,
		EventTypeInvestmentExpansion, EventTypeOperationalIncident, EventTypeLaborRestructuring,
		EventTypeMacro, EventTypeGeopolitics,
	}
}

// EventDirection is the direction of a tagged event.
type EventDirection string

// Known event directions.
const (
	EventDirectionPositive EventDirection = "positive"
	EventDirectionNegative EventDirection = "negative"
	EventDirectionNeutral  EventDirection = "neutral"
	EventDirectionUnclear  EventDirection = "unclear"
)

// EventDirectionValues returns the known event directions.
func EventDirectionValues() []EventDirection {
	return []EventDirection{EventDirectionPositive, EventDirectionNegative, EventDirectionNeutral, EventDirectionUnclear}
}

// Importance is the ordinal importance of an event (low < medium < high < critical).
type Importance string

// Known importance levels, in ascending order.
const (
	ImportanceLow      Importance = "low"
	ImportanceMedium   Importance = "medium"
	ImportanceHigh     Importance = "high"
	ImportanceCritical Importance = "critical"
)

// ImportanceValues returns the known importance levels in ascending order.
func ImportanceValues() []Importance {
	return []Importance{ImportanceLow, ImportanceMedium, ImportanceHigh, ImportanceCritical}
}

// SentimentLabel is the label of a sentiment block.
type SentimentLabel string

// Known sentiment labels.
const (
	SentimentLabelPositive SentimentLabel = "positive"
	SentimentLabelNegative SentimentLabel = "negative"
	SentimentLabelNeutral  SentimentLabel = "neutral"
)

// SentimentLabelValues returns the known sentiment labels.
func SentimentLabelValues() []SentimentLabel {
	return []SentimentLabel{SentimentLabelPositive, SentimentLabelNegative, SentimentLabelNeutral}
}

// SourceType is the type of a source (source.type of an article, type of a
// /v3/sources entry). Both are nullable: a source may be unclassified.
type SourceType string

// Known source types.
const (
	SourceTypeNewswire       SourceType = "newswire"
	SourceTypePressRelease   SourceType = "press_release"
	SourceTypeFinancialMedia SourceType = "financial_media"
	SourceTypeGeneralMedia   SourceType = "general_media"
	SourceTypeAggregator     SourceType = "aggregator"
	SourceTypeBlog           SourceType = "blog"
	SourceTypeGovernment     SourceType = "government"
)

// SourceTypeValues returns the known source types.
func SourceTypeValues() []SourceType {
	return []SourceType{
		SourceTypeNewswire, SourceTypePressRelease, SourceTypeFinancialMedia, SourceTypeGeneralMedia,
		SourceTypeAggregator, SourceTypeBlog, SourceTypeGovernment,
	}
}

// StreamEventName is the wire name of a WebSocket frame.
type StreamEventName string

// Known stream event names.
const (
	StreamEventNameArticleCreated      StreamEventName = "article.created"
	StreamEventNameArticleUpdated      StreamEventName = "article.updated"
	StreamEventNameConnectionAdmitted  StreamEventName = "connection.admitted"
	StreamEventNameConnectionPong      StreamEventName = "connection.pong"
	StreamEventNameConnectionPreempted StreamEventName = "connection.preempted"
	StreamEventNameConnectionError     StreamEventName = "connection.error"
)

// StreamEventNameValues returns every known stream event name.
func StreamEventNameValues() []StreamEventName {
	return append(StreamEventNameArticleEvents(), StreamEventNameConnectionEvents()...)
}

// StreamEventNameArticleEvents returns the known article event names.
func StreamEventNameArticleEvents() []StreamEventName {
	return []StreamEventName{StreamEventNameArticleCreated, StreamEventNameArticleUpdated}
}

// StreamEventNameConnectionEvents returns the known connection event names.
func StreamEventNameConnectionEvents() []StreamEventName {
	return []StreamEventName{
		StreamEventNameConnectionAdmitted, StreamEventNameConnectionPong,
		StreamEventNameConnectionPreempted, StreamEventNameConnectionError,
	}
}

// ConnectionErrorCode is the code of a connection.error frame.
type ConnectionErrorCode string

// Known connection.error codes.
const (
	ConnectionErrorCodePingOnDefaultRoute ConnectionErrorCode = "PING_ON_DEFAULT_ROUTE"
	ConnectionErrorCodeHandshakeFailed    ConnectionErrorCode = "HANDSHAKE_FAILED"
	ConnectionErrorCodeValidationFailed   ConnectionErrorCode = "VALIDATION_FAILED"
	ConnectionErrorCodeSearchFailed       ConnectionErrorCode = "SEARCH_FAILED"
	ConnectionErrorCodeInternalError      ConnectionErrorCode = "INTERNAL_ERROR"
)

// ConnectionErrorCodeValues returns the known connection.error codes.
func ConnectionErrorCodeValues() []ConnectionErrorCode {
	return []ConnectionErrorCode{
		ConnectionErrorCodePingOnDefaultRoute, ConnectionErrorCodeHandshakeFailed,
		ConnectionErrorCodeValidationFailed, ConnectionErrorCodeSearchFailed, ConnectionErrorCodeInternalError,
	}
}

// PreemptReason is the reason of a connection.preempted frame.
type PreemptReason string

// Known preempt reasons.
const (
	PreemptReasonConnectionLimit PreemptReason = "connection_limit"
	PreemptReasonQuotaExceeded   PreemptReason = "quota_exceeded"
)

// PreemptReasonValues returns the known preempt reasons.
func PreemptReasonValues() []PreemptReason {
	return []PreemptReason{PreemptReasonConnectionLimit, PreemptReasonQuotaExceeded}
}

// WebhookEventName is the event name of a webhook envelope.
type WebhookEventName string

// Known webhook event names.
const (
	WebhookEventNameArticleCreated WebhookEventName = "article.created"
	WebhookEventNameArticleUpdated WebhookEventName = "article.updated"
)

// WebhookEventNameValues returns the known webhook event names.
func WebhookEventNameValues() []WebhookEventName {
	return []WebhookEventName{WebhookEventNameArticleCreated, WebhookEventNameArticleUpdated}
}

// ProblemCode is the code member of an API error.
type ProblemCode string

// Known problem codes.
const (
	ProblemCodeValidationFailed ProblemCode = "VALIDATION_FAILED"
	ProblemCodeCursorInvalid    ProblemCode = "CURSOR_INVALID"
	ProblemCodeCursorMismatch   ProblemCode = "CURSOR_MISMATCH"
	ProblemCodeUnauthorized     ProblemCode = "UNAUTHORIZED"
	ProblemCodeForbidden        ProblemCode = "FORBIDDEN"
	ProblemCodeNotFound         ProblemCode = "NOT_FOUND"
	ProblemCodeRateLimited      ProblemCode = "RATE_LIMITED"
	ProblemCodeInternalError    ProblemCode = "INTERNAL_ERROR"
)

// ProblemCodeValues returns the known problem codes.
func ProblemCodeValues() []ProblemCode {
	return []ProblemCode{
		ProblemCodeValidationFailed, ProblemCodeCursorInvalid, ProblemCodeCursorMismatch,
		ProblemCodeUnauthorized, ProblemCodeForbidden, ProblemCodeNotFound,
		ProblemCodeRateLimited, ProblemCodeInternalError,
	}
}

// RateLimitType tells a monthly quota from a burst limit on a 429.
type RateLimitType string

// Known rate limit types. RateLimitTypeUnknown is set by the SDK when the server
// sends neither a limitType nor a known detail text.
const (
	RateLimitTypeMonthly RateLimitType = "monthly"
	RateLimitTypeBurst   RateLimitType = "burst"
	RateLimitTypeUnknown RateLimitType = "unknown"
)

// RateLimitTypeValues returns the known rate limit types, unknown included.
func RateLimitTypeValues() []RateLimitType {
	return []RateLimitType{RateLimitTypeMonthly, RateLimitTypeBurst, RateLimitTypeUnknown}
}

// StreamEventKind is the kind of a stream item.
type StreamEventKind string

// Stream item kinds.
const (
	StreamEventKindCreated StreamEventKind = "created"
	StreamEventKindUpdated StreamEventKind = "updated"
	StreamEventKindOther   StreamEventKind = "other"
)

// ReconnectReason says why a stream opened a new connection.
type ReconnectReason string

// Known reconnect reasons.
const (
	ReconnectReasonRotation        ReconnectReason = "rotation"
	ReconnectReasonWatchdog        ReconnectReason = "watchdog"
	ReconnectReasonClosed          ReconnectReason = "closed"
	ReconnectReasonHandshakeFailed ReconnectReason = "handshake_failed"
	ReconnectReasonSearchFailed    ReconnectReason = "search_failed"
	ReconnectReasonDialFailed      ReconnectReason = "dial_failed"
	ReconnectReasonDialRefused     ReconnectReason = "dial_refused"
	ReconnectReasonConnectionLimit ReconnectReason = "connection_limit"
	ReconnectReasonRateLimited     ReconnectReason = "rate_limited"
	ReconnectReasonQuotaExceeded   ReconnectReason = "quota_exceeded"
)

// ReconnectReasonValues returns the known reconnect reasons.
func ReconnectReasonValues() []ReconnectReason {
	return []ReconnectReason{
		ReconnectReasonRotation, ReconnectReasonWatchdog, ReconnectReasonClosed,
		ReconnectReasonHandshakeFailed, ReconnectReasonSearchFailed, ReconnectReasonDialFailed,
		ReconnectReasonDialRefused, ReconnectReasonConnectionLimit, ReconnectReasonRateLimited,
		ReconnectReasonQuotaExceeded,
	}
}
