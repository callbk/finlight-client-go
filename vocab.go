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

// SourceType is the type of a source. The server sends null today.
type SourceType string

// SourceTypeValues returns the known source types (none today).
func SourceTypeValues() []SourceType { return []SourceType{} }

// StreamEventName is the wire name of a WebSocket frame.
type StreamEventName string

// Known stream event names.
const (
	StreamEventArticleCreated      StreamEventName = "article.created"
	StreamEventArticleUpdated      StreamEventName = "article.updated"
	StreamEventConnectionAdmitted  StreamEventName = "connection.admitted"
	StreamEventConnectionPong      StreamEventName = "connection.pong"
	StreamEventConnectionPreempted StreamEventName = "connection.preempted"
	StreamEventConnectionError     StreamEventName = "connection.error"
)

// StreamEventNameValues returns every known stream event name.
func StreamEventNameValues() []StreamEventName {
	return append(StreamEventNameArticleEvents(), StreamEventNameConnectionEvents()...)
}

// StreamEventNameArticleEvents returns the known article event names.
func StreamEventNameArticleEvents() []StreamEventName {
	return []StreamEventName{StreamEventArticleCreated, StreamEventArticleUpdated}
}

// StreamEventNameConnectionEvents returns the known connection event names.
func StreamEventNameConnectionEvents() []StreamEventName {
	return []StreamEventName{
		StreamEventConnectionAdmitted, StreamEventConnectionPong,
		StreamEventConnectionPreempted, StreamEventConnectionError,
	}
}

// ConnectionErrorCode is the code of a connection.error frame.
type ConnectionErrorCode string

// Known connection.error codes.
const (
	ConnectionErrorPingOnDefaultRoute ConnectionErrorCode = "PING_ON_DEFAULT_ROUTE"
	ConnectionErrorHandshakeFailed    ConnectionErrorCode = "HANDSHAKE_FAILED"
	ConnectionErrorValidationFailed   ConnectionErrorCode = "VALIDATION_FAILED"
	ConnectionErrorSearchFailed       ConnectionErrorCode = "SEARCH_FAILED"
	ConnectionErrorInternalError      ConnectionErrorCode = "INTERNAL_ERROR"
)

// ConnectionErrorCodeValues returns the known connection.error codes.
func ConnectionErrorCodeValues() []ConnectionErrorCode {
	return []ConnectionErrorCode{
		ConnectionErrorPingOnDefaultRoute, ConnectionErrorHandshakeFailed,
		ConnectionErrorValidationFailed, ConnectionErrorSearchFailed, ConnectionErrorInternalError,
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
	WebhookEventArticleCreated WebhookEventName = "article.created"
	WebhookEventArticleUpdated WebhookEventName = "article.updated"
)

// WebhookEventNameValues returns the known webhook event names.
func WebhookEventNameValues() []WebhookEventName {
	return []WebhookEventName{WebhookEventArticleCreated, WebhookEventArticleUpdated}
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

// Known rate limit types. RateLimitUnknown is set by the SDK when the server
// sends neither a limitType nor a known detail text.
const (
	RateLimitMonthly RateLimitType = "monthly"
	RateLimitBurst   RateLimitType = "burst"
	RateLimitUnknown RateLimitType = "unknown"
)

// RateLimitTypeValues returns the known rate limit types, unknown included.
func RateLimitTypeValues() []RateLimitType {
	return []RateLimitType{RateLimitMonthly, RateLimitBurst, RateLimitUnknown}
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
	ReconnectRotation        ReconnectReason = "rotation"
	ReconnectWatchdog        ReconnectReason = "watchdog"
	ReconnectClosed          ReconnectReason = "closed"
	ReconnectHandshakeFailed ReconnectReason = "handshake_failed"
	ReconnectSearchFailed    ReconnectReason = "search_failed"
	ReconnectDialFailed      ReconnectReason = "dial_failed"
	ReconnectDialRefused     ReconnectReason = "dial_refused"
	ReconnectConnectionLimit ReconnectReason = "connection_limit"
	ReconnectRateLimited     ReconnectReason = "rate_limited"
	ReconnectQuotaExceeded   ReconnectReason = "quota_exceeded"
)
