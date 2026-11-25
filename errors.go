package finlight

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Error is implemented by every error the SDK returns, except a cancelled
// context (that arrives as ctx.Err() unchanged) and a webhook body of the wrong
// payload version (see the webhook package). Kind tells the error types apart
// without a type switch.
type Error interface {
	error
	Kind() ErrorKind
}

// ErrorKind names the kind of an SDK error.
type ErrorKind string

// Error kinds.
const (
	ErrorKindAPI                 ErrorKind = "api"
	ErrorKindValidation          ErrorKind = "validation"
	ErrorKindNetwork             ErrorKind = "network"
	ErrorKindTimeout             ErrorKind = "timeout"
	ErrorKindDecode              ErrorKind = "decode"
	ErrorKindWebhookVerification ErrorKind = "webhook_verification"
	ErrorKindPreempted           ErrorKind = "preempted"
)

// APIErrorKind is the kind of an API error, derived from its code (or, without
// a code, from the HTTP status).
type APIErrorKind string

// API error kinds.
const (
	APIKindValidationFailed APIErrorKind = "validation_failed"
	APIKindCursorInvalid    APIErrorKind = "cursor_invalid"
	APIKindCursorMismatch   APIErrorKind = "cursor_mismatch"
	APIKindUnauthorized     APIErrorKind = "unauthorized"
	APIKindForbidden        APIErrorKind = "forbidden"
	APIKindNotFound         APIErrorKind = "not_found"
	APIKindRateLimited      APIErrorKind = "rate_limited"
	APIKindInternalError    APIErrorKind = "internal_error"
	APIKindOther            APIErrorKind = "other"
)

// FieldIssue is one entry of APIError.Errors.
type FieldIssue struct {
	Field         string            // the request field, e.g. "eventTypes[0]"
	Issue         string            // human-readable text, not a stable value
	AllowedValues []string          // set for closed vocabularies, else nil
	Constraints   map[string]string // only on stream errors: {rule: text}
}

// APIError is every error the API answers with: REST error responses, refused
// WebSocket dials and terminal connection.error frames. APIKind names the
// kind; for a 429, LimitType and RetryAfter are set.
type APIError struct {
	Status    int          // HTTP status (0 for a connection.error frame)
	Code      ProblemCode  // problem code, "" when the body carries none
	Type      string       // problem type URI, "" when absent
	Title     string       // problem title
	Detail    string       // problem detail or fallback message
	RequestID string       // the server's requestId, else the X-Request-Id the SDK sent
	Errors    []FieldIssue // field issues, nil when the body has none
	APIKind   APIErrorKind // kind derived from the code or status
	Raw       string       // the error body as received (text)

	// LimitType is set on a 429: monthly, burst, unknown or the value the
	// server sent. Empty on other statuses.
	LimitType RateLimitType
	// RetryAfter is the Retry-After header of a 429 (0 when the header is absent).
	RetryAfter time.Duration

	rawJSON json.RawMessage
	cause   error
}

// Kind reports ErrorKindAPI.
func (e *APIError) Kind() ErrorKind { return ErrorKindAPI }

// Unwrap returns the underlying cause, if any.
func (e *APIError) Unwrap() error { return e.cause }

// RawJSON returns the parsed error body as JSON text, or nil when the body was
// not JSON.
func (e *APIError) RawJSON() json.RawMessage { return e.rawJSON }

func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("finlight: API error")
	if e.Status != 0 {
		fmt.Fprintf(&b, " %d", e.Status)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	}
	if e.Detail != "" {
		fmt.Fprintf(&b, ": %s", e.Detail)
	} else if e.Title != "" {
		fmt.Fprintf(&b, ": %s", e.Title)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request %s)", e.RequestID)
	}
	return b.String()
}

// ValidationError is a local check that failed before any request or dial.
type ValidationError struct {
	Param   string // the rejected parameter, "" when none applies
	Message string
}

// Kind reports ErrorKindValidation.
func (e *ValidationError) Kind() ErrorKind { return ErrorKindValidation }

func (e *ValidationError) Error() string { return "finlight: invalid argument: " + e.Message }

// NetworkError is a transport failure (DNS, connect, reset, TLS, ...).
type NetworkError struct {
	Message string
	Err     error // the transport error
}

// Kind reports ErrorKindNetwork.
func (e *NetworkError) Kind() ErrorKind { return ErrorKindNetwork }

// Unwrap returns the transport error.
func (e *NetworkError) Unwrap() error { return e.Err }

func (e *NetworkError) Error() string {
	if e.Err != nil {
		return "finlight: network error: " + e.Message + ": " + e.Err.Error()
	}
	return "finlight: network error: " + e.Message
}

// TimeoutError is a network error caused by a timeout of one attempt (or of
// the total timeout). errors.As also finds it as a *NetworkError.
type TimeoutError struct {
	*NetworkError
}

// Kind reports ErrorKindTimeout.
func (e *TimeoutError) Kind() ErrorKind { return ErrorKindTimeout }

func (e *TimeoutError) Error() string {
	return strings.Replace(e.NetworkError.Error(), "network error", "timeout", 1)
}

// As lets errors.As match a *TimeoutError as a *NetworkError.
func (e *TimeoutError) As(target any) bool {
	if t, ok := target.(**NetworkError); ok {
		*t = e.NetworkError
		return true
	}
	return false
}

// DecodeError is a response, frame or webhook body that could not be read.
type DecodeError struct {
	Path    string // JSON path of the value that did not fit, "" when unknown
	Raw     string // the value that did not fit (may be cut short)
	Message string
	Err     error
}

// Kind reports ErrorKindDecode.
func (e *DecodeError) Kind() ErrorKind { return ErrorKindDecode }

// Unwrap returns the underlying cause, if any.
func (e *DecodeError) Unwrap() error { return e.Err }

func (e *DecodeError) Error() string {
	if e.Path != "" {
		return "finlight: decode error at " + e.Path + ": " + e.Message
	}
	return "finlight: decode error: " + e.Message
}

// PreemptedError ends a stream when the server preempted its connection.
type PreemptedError struct {
	Reason     PreemptReason
	NewLeaseID *string // lease of the connection that took over, nil when none
	Timestamp  int64   // epoch milliseconds
}

// Kind reports ErrorKindPreempted.
func (e *PreemptedError) Kind() ErrorKind { return ErrorKindPreempted }

func (e *PreemptedError) Error() string {
	return "finlight: connection preempted: " + string(e.Reason)
}

// Interface checks.
var (
	_ Error = (*APIError)(nil)
	_ Error = (*ValidationError)(nil)
	_ Error = (*NetworkError)(nil)
	_ Error = (*TimeoutError)(nil)
	_ Error = (*DecodeError)(nil)
	_ Error = (*PreemptedError)(nil)
)

func validationErr(param, format string, args ...any) *ValidationError {
	return &ValidationError{Param: param, Message: fmt.Sprintf(format, args...)}
}

// apiKindForCode maps a problem code to its kind; unknown codes are "other".
func apiKindForCode(code ProblemCode) APIErrorKind {
	switch code {
	case ProblemCodeValidationFailed:
		return APIKindValidationFailed
	case ProblemCodeCursorInvalid:
		return APIKindCursorInvalid
	case ProblemCodeCursorMismatch:
		return APIKindCursorMismatch
	case ProblemCodeUnauthorized:
		return APIKindUnauthorized
	case ProblemCodeForbidden:
		return APIKindForbidden
	case ProblemCodeNotFound:
		return APIKindNotFound
	case ProblemCodeRateLimited:
		return APIKindRateLimited
	case ProblemCodeInternalError:
		return APIKindInternalError
	}
	return APIKindOther
}

// apiKindForStatus maps an HTTP status to a kind, for bodies without a code.
func apiKindForStatus(status int) APIErrorKind {
	switch {
	case status == 400:
		return APIKindValidationFailed
	case status == 401:
		return APIKindUnauthorized
	case status == 403:
		return APIKindForbidden
	case status == 404:
		return APIKindNotFound
	case status == 429:
		return APIKindRateLimited
	case status >= 500:
		return APIKindInternalError
	}
	return APIKindOther
}
