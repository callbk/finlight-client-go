package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func kitBody(t *testing.T, rel string) ([]byte, string) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/conformance/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Secret  string `json:"secret"`
		Request struct {
			RawBodyBase64 string `json:"rawBodyBase64"`
		} `json:"request"`
	}
	_ = json.Unmarshal(data, &f)
	body, _ := base64.StdEncoding.DecodeString(f.Request.RawBodyBase64)
	return body, f.Secret
}

func post(t *testing.T, h http.Handler, body []byte, secret string) int {
	t.Helper()
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Timestamp", ts)
	req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(m.Sum(nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestWebhookEndpoint(t *testing.T) {
	body, secret := kitBody(t, "fixtures/webhook/created-enriched.json")
	v2, _ := kitBody(t, "fixtures/webhook/v2-body.json")
	work := make(chan *finlight.WebhookEvent, 10)
	h := newHandler(secret, work)

	if code := post(t, h, body, secret); code != 200 {
		t.Fatalf("event: %d", code)
	}
	if ev := <-work; ev.EventID != "evt_9_Rx6TwzctfS0RigczzhjK" {
		t.Errorf("event = %s", ev.EventID)
	}
	if code := post(t, h, body, secret); code != 200 || len(work) != 0 {
		t.Errorf("duplicate: %d, queued %d", code, len(work))
	}
	if code := post(t, h, v2, secret); code != 200 {
		t.Errorf("v2 body: %d, want 2xx", code)
	}
	if code := post(t, h, body, "wrong-secret"); code != 400 {
		t.Errorf("bad signature: %d, want 400", code)
	}
}
