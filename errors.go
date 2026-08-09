package finlight

import (
	"errors"
	"fmt"
)

// ErrBlocked is returned by the WebSocket clients' Err method when the server
// permanently rejected the connection (close code 1008). Reconnecting will not
// help; contact finlight support.
var ErrBlocked = errors.New("finlight: connection rejected by server (blocked)")

// APIError is returned for non-2xx REST responses (after retries).
type APIError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("finlight: API error: %s", e.Status)
}

// WebhookVerificationError is returned by ConstructWebhookEvent when a webhook
// fails signature, timestamp, or payload validation.
type WebhookVerificationError struct {
	msg string
}

func (e *WebhookVerificationError) Error() string { return e.msg }
