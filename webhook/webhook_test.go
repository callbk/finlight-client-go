package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"testing"
	"time"

	finlight "github.com/callbk/finlight-client-go/v2"
)

const testSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var testNow = time.Date(2026, 11, 23, 21, 4, 5, 123e6, time.UTC)

func sign(body []byte, ts, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func signed(body string) (http.Header, []byte) {
	ts := testNow.Format("2006-01-02T15:04:05.000Z")
	h := http.Header{}
	h.Set("X-Webhook-Timestamp", ts)
	h.Set("X-Webhook-Signature", sign([]byte(body), ts, testSecret))
	return h, []byte(body)
}

func construct(h http.Header, body []byte) (*finlight.WebhookEvent, error) {
	return ConstructEvent(body, h, testSecret, WithNow(func() time.Time { return testNow }))
}

const article = `{"id":"a","link":"https://x.example/1","title":"t","source":{"domain":"x.example","type":null}}`

func TestPayloadVersionRules(t *testing.T) {
	cases := []struct {
		name    string
		pv      string
		v3      bool
		present bool
	}{
		{"number 3", `3`, true, true},
		{"3.0 counts as 3", `3.0`, true, true},
		{"string 3", `"3"`, false, true},
		{"bool", `true`, false, true},
		{"2", `2`, false, true},
		{"4", `4`, false, true},
		{"null", `null`, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, body := signed(`{"eventId":"evt_1","event":"article.created","payloadVersion":` + c.pv + `,"data":` + article + `}`)
			ev, err := construct(h, body)
			if c.v3 {
				if err != nil || ev.Data.ID != "a" {
					t.Fatalf("v3 body: %v", err)
				}
				return
			}
			var w *WrongPayloadVersionError
			if !errors.As(err, &w) || w.DetectedVersion != 2 || string(w.PayloadVersion) != c.pv || !w.PayloadVersionPresent {
				t.Fatalf("err = %#v", err)
			}
			if w.Link != "https://x.example/1" {
				t.Errorf("link = %q", w.Link)
			}
		})
	}
}

func TestTamperedV2BodyIsASignatureError(t *testing.T) {
	h, body := signed(`{"link":"https://x.example/1","title":"t"}`)
	body[2] = 'L'
	_, err := construct(h, body)
	var ve *VerificationError
	if !errors.As(err, &ve) || ve.Reason != ReasonSignatureMismatch {
		t.Fatalf("err = %v", err)
	}
}

func TestBodyThatIsNotAnObject(t *testing.T) {
	for _, b := range []string{`[1,2]`, `"x"`, `not json`} {
		h, body := signed(b)
		_, err := construct(h, body)
		var de *finlight.DecodeError
		if !errors.As(err, &de) || de.Kind() != finlight.ErrorKindDecode {
			t.Errorf("%s: err = %v", b, err)
		}
	}
	// A v3 envelope whose article lacks link is a decode error, never half an event.
	h, body := signed(`{"eventId":"e","event":"article.created","payloadVersion":3,"data":{"id":"a","title":"t","source":{"domain":"d"}}}`)
	_, err := construct(h, body)
	var de *finlight.DecodeError
	if !errors.As(err, &de) || de.Path != "data.link" {
		t.Errorf("err = %v", err)
	}
}

func TestHeaderProblems(t *testing.T) {
	h, body := signed(article)
	dup := h.Clone()
	dup["x-webhook-signature"] = []string{h.Get("X-Webhook-Signature")}
	if _, err := construct(dup, body); !hasReason(err, ReasonDuplicateHeader) {
		t.Errorf("duplicate header: %v", err)
	}
	twice := h.Clone()
	twice.Add("X-Webhook-Timestamp", h.Get("X-Webhook-Timestamp"))
	if _, err := construct(twice, body); !hasReason(err, ReasonDuplicateHeader) {
		t.Errorf("repeated header: %v", err)
	}
	nonASCII := h.Clone()
	nonASCII.Set("X-Webhook-Timestamp", "2026-11-23T21:04:05.123Ž")
	if _, err := construct(nonASCII, body); !hasReason(err, ReasonNonASCIIHeader) {
		t.Errorf("non-ASCII: %v", err)
	}
	noZone := h.Clone()
	noZone.Set("X-Webhook-Timestamp", "2026-11-23T21:04:05.123")
	if _, err := construct(noZone, body); !hasReason(err, ReasonUnparseableTimestamp) {
		t.Errorf("no zone: %v", err)
	}
	if _, err := ConstructEvent(body, h, ""); !hasReason(err, ReasonInvalidSecret) {
		t.Errorf("empty secret: %v", err)
	}
	if _, err := ConstructEvent(nil, h, testSecret); !hasReason(err, ReasonInvalidRawBody) {
		t.Errorf("nil body: %v", err)
	}
}

func TestFromValuesAndVerifySignature(t *testing.T) {
	h, body := signed(`{"eventId":"evt_test_1","event":"article.created","payloadVersion":3,"data":` + article + `}`)
	now := WithNow(func() time.Time { return testNow })
	ev, err := ConstructEventFromValues(body, h.Get("X-Webhook-Signature"), h.Get("X-Webhook-Timestamp"), testSecret, now)
	if err != nil || !ev.IsTest {
		t.Fatalf("from values: %v", err)
	}
	if err := VerifySignature(body, h.Get("X-Webhook-Signature"), h.Get("X-Webhook-Timestamp"), testSecret, now); err != nil {
		t.Fatal(err)
	}
	// Default tolerance is 300 s, both directions.
	late := WithNow(func() time.Time { return testNow.Add(301 * time.Second) })
	if err := VerifySignature(body, h.Get("X-Webhook-Signature"), h.Get("X-Webhook-Timestamp"), testSecret, late); !hasReason(err, ReasonTimestampOutsideTolerance) {
		t.Errorf("late: %v", err)
	}
	if DefaultTolerance != 300*time.Second {
		t.Error("DefaultTolerance")
	}
}

func hasReason(err error, r Reason) bool {
	var ve *VerificationError
	return errors.As(err, &ve) && ve.Reason == r
}
