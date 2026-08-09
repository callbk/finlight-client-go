package finlight

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	signaturePrefix = "sha256="
	replayTolerance = 5 * time.Minute
)

// webhookTimestampLayouts cover RFC 3339 and Python isoformat timestamps.
var webhookTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
}

// ConstructWebhookEvent verifies a finlight webhook and returns the contained
// article.
//
// rawBody must be the unmodified request body. signature is the value of the
// X-Webhook-Signature header (with or without the "sha256=" prefix).
// endpointSecret is your webhook secret from the finlight dashboard.
// timestamp is the X-Webhook-Timestamp header; pass "" if the webhook has
// none, otherwise it is included in the signed message and checked against a
// 5-minute replay tolerance.
func ConstructWebhookEvent(rawBody []byte, signature, endpointSecret, timestamp string) (*Article, error) {
	signature = strings.TrimPrefix(signature, signaturePrefix)

	message := rawBody
	if timestamp != "" {
		message = append([]byte(timestamp+"."), rawBody...)
	}
	mac := hmac.New(sha256.New, []byte(endpointSecret))
	mac.Write(message)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, &WebhookVerificationError{msg: "Invalid webhook signature"}
	}

	if timestamp != "" {
		if err := verifyWebhookTimestamp(timestamp); err != nil {
			return nil, err
		}
	}

	var article Article
	if err := json.Unmarshal(rawBody, &article); err != nil {
		return nil, &WebhookVerificationError{msg: "Invalid JSON payload"}
	}
	return &article, nil
}

func verifyWebhookTimestamp(timestamp string) error {
	var parsed time.Time
	var err error
	for _, layout := range webhookTimestampLayouts {
		if parsed, err = time.Parse(layout, timestamp); err == nil {
			break
		}
	}
	if err != nil {
		return &WebhookVerificationError{msg: "Invalid timestamp format"}
	}
	if d := time.Since(parsed); d > replayTolerance || d < -replayTolerance {
		return &WebhookVerificationError{msg: "Webhook timestamp outside allowed tolerance"}
	}
	return nil
}
