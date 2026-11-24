package finlight

import (
	"encoding/json"
	"strings"
	"time"
)

// WebhookEvent is a verified v3 webhook envelope (see the webhook package).
// ChangedFields is nil when absent (it is only sent on article.updated).
type WebhookEvent struct {
	EventID        string           `json:"eventId"` // idempotency key: dedupe on it
	Event          WebhookEventName `json:"event"`   // open
	OccurredAt     string           `json:"occurredAt"`
	PayloadVersion int              `json:"payloadVersion"`
	SubscriptionID string           `json:"subscriptionId"`
	ChangedFields  []string         `json:"changedFields,omitempty"`
	Data           Article          `json:"data"`
	// IsTest is true for the dashboard test webhook (eventId starts with
	// "evt_test"). Test webhooks are not cut by entitlements.
	IsTest bool `json:"-"`

	raw json.RawMessage
}

// RawJSON returns the envelope exactly as received.
func (e *WebhookEvent) RawJSON() json.RawMessage { return e.raw }

// OccurredAtTime parses OccurredAt strictly as RFC 3339.
func (e *WebhookEvent) OccurredAtTime() (time.Time, bool) { return parseTimeString(e.OccurredAt) }

// UnmarshalJSON decodes an envelope with exact key matching. It does not check
// payloadVersion; use webhook.ConstructEvent for received webhooks.
func (e *WebhookEvent) UnmarshalJSON(b []byte) error {
	v, err := decodeWebhookEvent(b)
	if err != nil {
		return err
	}
	*e = *v
	return nil
}

func decodeWebhookEvent(raw json.RawMessage) (*WebhookEvent, error) {
	o, err := parseObject(raw, "")
	if err != nil {
		return nil, err
	}
	e := &WebhookEvent{raw: append(json.RawMessage(nil), raw...)}
	if e.EventID, err = o.plainString("", "eventId"); err != nil {
		return nil, err
	}
	ev, err := o.plainString("", "event")
	if err != nil {
		return nil, err
	}
	e.Event = WebhookEventName(ev)
	if e.OccurredAt, err = o.plainString("", "occurredAt"); err != nil {
		return nil, err
	}
	if pv, ok := o["payloadVersion"]; ok && jsonKind(pv) == "number" {
		var f float64
		if json.Unmarshal(pv, &f) == nil {
			e.PayloadVersion = int(f)
		}
	}
	if e.SubscriptionID, err = o.plainString("", "subscriptionId"); err != nil {
		return nil, err
	}
	if _, ok := o["changedFields"]; ok {
		if e.ChangedFields, err = o.stringList("", "changedFields"); err != nil {
			return nil, err
		}
	}
	data, ok := o["data"]
	if !ok {
		return nil, decodeErr("data", nil, "required key is missing")
	}
	a, err := decodeArticle(data, "data")
	if err != nil {
		return nil, err
	}
	e.Data = *a
	e.IsTest = strings.HasPrefix(e.EventID, "evt_test")
	return e, nil
}
