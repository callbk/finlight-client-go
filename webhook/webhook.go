// Package webhook verifies and reads finlight webhooks on your own server.
//
// Read the raw request body before any framework parses it, then call
// ConstructEvent. Answer with 2xx within 5 seconds and do slow work afterwards;
// dedupe on WebhookEvent.EventID. Never answer with 410: it disables the
// webhook at once.
//
// After a switch to payload version 3, retries queued before the switch still
// arrive in the old format for about 2 hours. ConstructEvent reports such a
// body as *WrongPayloadVersionError (find it with errors.As). Answer it with
// 2xx and keep its Raw JSON, so that the delivery does not count as a failure.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	finlight "github.com/callbk/finlight-client-go/v2"
)

// DefaultTolerance is the default allowed distance between the
// X-Webhook-Timestamp header and now, in both directions.
const DefaultTolerance = 300 * time.Second

// Header names (looked up case-insensitively).
const (
	signatureHeader = "X-Webhook-Signature"
	timestampHeader = "X-Webhook-Timestamp"
)

// Reason says why a webhook failed verification.
type Reason string

// Verification failure reasons.
const (
	ReasonInvalidRawBody             Reason = "invalid_raw_body"
	ReasonInvalidSecret              Reason = "invalid_secret"
	ReasonMissingSignature           Reason = "missing_signature"
	ReasonMissingTimestamp           Reason = "missing_timestamp"
	ReasonDuplicateHeader            Reason = "duplicate_header"
	ReasonNonASCIIHeader             Reason = "non_ascii_header"
	ReasonUnsupportedSignatureScheme Reason = "unsupported_signature_scheme"
	ReasonUnparseableTimestamp       Reason = "unparseable_timestamp"
	ReasonSignatureMismatch          Reason = "signature_mismatch"
	ReasonTimestampOutsideTolerance  Reason = "timestamp_outside_tolerance"
)

// VerificationError is returned when a webhook fails verification. Answer it
// with 400 (never 410).
type VerificationError struct {
	Reason  Reason
	Message string
}

// Kind reports finlight.ErrorKindWebhookVerification.
func (e *VerificationError) Kind() finlight.ErrorKind { return finlight.ErrorKindWebhookVerification }

func (e *VerificationError) Error() string {
	return "finlight: webhook verification failed (" + string(e.Reason) + "): " + e.Message
}

var _ finlight.Error = (*VerificationError)(nil)

// WrongPayloadVersionError reports a correctly signed body that is not a v3
// envelope (payloadVersion is not the JSON number 3). It is not a
// verification failure: answer it with 2xx and keep Raw.
type WrongPayloadVersionError struct {
	// DetectedVersion follows the server's rule: 3 only for the number 3, else 2.
	DetectedVersion int
	// PayloadVersion is the payloadVersion value as received (nil when absent).
	PayloadVersion json.RawMessage
	// PayloadVersionPresent reports whether the key was in the body.
	PayloadVersionPresent bool
	// Raw is the parsed body.
	Raw map[string]any
	// Link is the article link when it can be read, else "".
	Link string

	body json.RawMessage
}

func (e *WrongPayloadVersionError) Error() string {
	got := "absent"
	if e.PayloadVersionPresent {
		got = string(e.PayloadVersion)
	}
	return "finlight: webhook body is not payload version 3 (payloadVersion " + got + "); answer it with 2xx"
}

// RawJSON returns the body exactly as received.
func (e *WrongPayloadVersionError) RawJSON() json.RawMessage { return e.body }

// Option configures verification.
type Option func(*options)

type options struct {
	tolerance time.Duration
	now       func() time.Time
}

// WithTolerance sets the allowed distance between the timestamp and now
// (default DefaultTolerance).
func WithTolerance(d time.Duration) Option { return func(o *options) { o.tolerance = d } }

// WithNow replaces the clock used for the tolerance check.
func WithNow(now func() time.Time) Option { return func(o *options) { o.now = now } }

func buildOptions(opts []Option) options {
	o := options{tolerance: DefaultTolerance, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.now == nil {
		o.now = time.Now
	}
	return o
}

// ConstructEvent verifies a webhook and decodes its envelope. body must be the
// raw request body exactly as received; header the request headers (names are
// matched case-insensitively). The order is: signature, timestamp, parse,
// payloadVersion, decode. Errors: *VerificationError (answer 400),
// *WrongPayloadVersionError (answer 2xx) or *finlight.DecodeError.
func ConstructEvent(body []byte, header http.Header, secret string, opts ...Option) (*finlight.WebhookEvent, error) {
	sig, err := headerValue(header, signatureHeader, ReasonMissingSignature)
	if err != nil {
		return nil, err
	}
	ts, err := headerValue(header, timestampHeader, ReasonMissingTimestamp)
	if err != nil {
		return nil, err
	}
	return ConstructEventFromValues(body, sig, ts, secret, opts...)
}

// ConstructEventFromValues is ConstructEvent with the two header values given
// directly.
func ConstructEventFromValues(body []byte, signature, timestamp, secret string, opts ...Option) (*finlight.WebhookEvent, error) {
	if err := VerifySignature(body, signature, timestamp, secret, opts...); err != nil {
		return nil, err
	}
	return decodeEnvelope(body)
}

// VerifySignature checks the signature and the timestamp of a webhook without
// decoding the body.
func VerifySignature(body []byte, signature, timestamp, secret string, opts ...Option) error {
	o := buildOptions(opts)
	if body == nil {
		return &VerificationError{Reason: ReasonInvalidRawBody, Message: "the raw body is required (nil given)"}
	}
	if secret == "" {
		return &VerificationError{Reason: ReasonInvalidSecret, Message: "the webhook secret must not be empty"}
	}
	if signature == "" {
		return &VerificationError{Reason: ReasonMissingSignature, Message: signatureHeader + " is missing"}
	}
	if timestamp == "" {
		return &VerificationError{Reason: ReasonMissingTimestamp, Message: timestampHeader + " is missing"}
	}
	if !isASCII(signature) || !isASCII(timestamp) {
		return &VerificationError{Reason: ReasonNonASCIIHeader, Message: "a webhook header holds non-ASCII characters"}
	}
	hexSig, ok := strings.CutPrefix(signature, "sha256=")
	if !ok {
		return &VerificationError{Reason: ReasonUnsupportedSignatureScheme, Message: "only sha256=<hex> signatures are accepted"}
	}
	ts, ok := parseTimestamp(timestamp)
	if !ok {
		return &VerificationError{Reason: ReasonUnparseableTimestamp, Message: "the timestamp is not RFC 3339 with a zone"}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(hexSig)
	if err != nil || len(got) != len(expected) || !hmac.Equal(got, expected) {
		return &VerificationError{Reason: ReasonSignatureMismatch, Message: "the signature does not match the raw body"}
	}
	if diff := o.now().Sub(ts); diff > o.tolerance || diff < -o.tolerance {
		return &VerificationError{Reason: ReasonTimestampOutsideTolerance, Message: "the timestamp is outside the tolerance"}
	}
	return nil
}

func decodeEnvelope(body []byte) (*finlight.WebhookEvent, error) {
	trimmed := bytes.TrimSpace(body)
	var obj map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &obj) != nil {
		return nil, &finlight.DecodeError{Message: "the webhook body is not a JSON object"}
	}
	pv, present := obj["payloadVersion"]
	if !present || !isNumberThree(pv) {
		w := &WrongPayloadVersionError{
			DetectedVersion:       2,
			PayloadVersionPresent: present,
			body:                  append(json.RawMessage(nil), body...),
		}
		if present {
			w.PayloadVersion = append(json.RawMessage(nil), pv...)
		}
		_ = json.Unmarshal(trimmed, &w.Raw)
		if l, ok := obj["link"]; ok {
			_ = json.Unmarshal(l, &w.Link)
		} else if d, ok := obj["data"]; ok {
			var data map[string]json.RawMessage
			if json.Unmarshal(d, &data) == nil {
				_ = json.Unmarshal(data["link"], &w.Link)
			}
		}
		return nil, w
	}
	var ev finlight.WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		var de *finlight.DecodeError
		if errors.As(err, &de) {
			return nil, de
		}
		return nil, &finlight.DecodeError{Message: "the webhook envelope could not be decoded", Err: err}
	}
	return &ev, nil
}

// isNumberThree follows the server check (v === 3): a JSON number equal to 3,
// so 3.0 counts; "3" and true do not.
func isNumberThree(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || (t[0] != '-' && (t[0] < '0' || t[0] > '9')) {
		return false
	}
	var f float64
	return json.Unmarshal(t, &f) == nil && f == 3
}

func headerValue(h http.Header, name string, missing Reason) (string, error) {
	var values []string
	for k, vs := range h {
		if strings.EqualFold(k, name) {
			values = append(values, vs...)
		}
	}
	switch len(values) {
	case 0:
		return "", &VerificationError{Reason: missing, Message: name + " is missing"}
	case 1:
		return values[0], nil
	}
	return "", &VerificationError{Reason: ReasonDuplicateHeader, Message: name + " is sent more than once"}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7e || (s[i] < 0x20 && s[i] != '\t') {
			return false
		}
	}
	return true
}

// parseTimestamp reads the header strictly as RFC 3339 with Z or an offset.
// Unix seconds and times without a zone are unparseable.
func parseTimestamp(s string) (time.Time, bool) {
	if strings.ContainsRune(s, ',') {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
