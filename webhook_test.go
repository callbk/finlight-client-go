package finlight

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

const webhookSecret = "test_secret_key"

const validWebhookPayload = `{
	"link": "https://example.com/article",
	"title": "Test Article",
	"publishDate": "2024-01-01T00:00:00Z",
	"source": "example.com",
	"language": "en",
	"sentiment": "positive",
	"confidence": "0.95",
	"summary": "This is a test article",
	"companies": [
		{"companyId": 1, "confidence": "0.90", "name": "Apple Inc.", "ticker": "AAPL", "exchange": "NASDAQ"}
	]
}`

func createSignature(payload, secret, timestamp string) string {
	message := payload
	if timestamp != "" {
		message = timestamp + "." + payload
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func assertVerificationError(t *testing.T, err error, wantMsg string) {
	t.Helper()
	var verr *WebhookVerificationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want WebhookVerificationError", err)
	}
	if verr.Error() != wantMsg {
		t.Errorf("error message = %q, want %q", verr.Error(), wantMsg)
	}
}

func TestWebhookValidSignatureAndTimestamp(t *testing.T) {
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	signature := "sha256=" + createSignature(validWebhookPayload, webhookSecret, timestamp)

	article, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "Test Article" || article.Link != "https://example.com/article" {
		t.Errorf("article = %+v", article)
	}
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !article.PublishDate.Time.Equal(want) {
		t.Errorf("publishDate = %v, want %v", article.PublishDate.Time, want)
	}
	if article.Confidence == nil || article.Confidence.Float64() != 0.95 {
		t.Errorf("confidence = %v, want 0.95", article.Confidence)
	}
	if len(article.Companies) != 1 || article.Companies[0].Confidence == nil ||
		article.Companies[0].Confidence.Float64() != 0.90 {
		t.Errorf("companies = %+v", article.Companies)
	}
}

func TestWebhookValidSignatureWithoutTimestamp(t *testing.T) {
	signature := "sha256=" + createSignature(validWebhookPayload, webhookSecret, "")

	article, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, "")
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "Test Article" {
		t.Errorf("title = %q", article.Title)
	}
	if article.Confidence == nil || article.Confidence.Float64() != 0.95 {
		t.Errorf("confidence = %v, want 0.95", article.Confidence)
	}
}

func TestWebhookSignatureWithoutPrefix(t *testing.T) {
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	signature := createSignature(validWebhookPayload, webhookSecret, timestamp)

	article, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "Test Article" {
		t.Errorf("title = %q", article.Title)
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := ConstructWebhookEvent([]byte(validWebhookPayload), "sha256=invalid_signature", webhookSecret, timestamp)
	assertVerificationError(t, err, "Invalid webhook signature")
}

func TestWebhookMismatchedSecret(t *testing.T) {
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	signature := "sha256=" + createSignature(validWebhookPayload, "wrong_secret", timestamp)

	_, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	assertVerificationError(t, err, "Invalid webhook signature")
}

func TestWebhookExpiredTimestamp(t *testing.T) {
	timestamp := time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339Nano)
	signature := "sha256=" + createSignature(validWebhookPayload, webhookSecret, timestamp)

	_, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	assertVerificationError(t, err, "Webhook timestamp outside allowed tolerance")
}

func TestWebhookTimestampWithinTolerance(t *testing.T) {
	timestamp := time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339Nano)
	signature := "sha256=" + createSignature(validWebhookPayload, webhookSecret, timestamp)

	article, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "Test Article" {
		t.Errorf("title = %q", article.Title)
	}
}

func TestWebhookInvalidJSONPayload(t *testing.T) {
	rawBody := "invalid json"
	signature := "sha256=" + createSignature(rawBody, webhookSecret, "")

	_, err := ConstructWebhookEvent([]byte(rawBody), signature, webhookSecret, "")
	assertVerificationError(t, err, "Invalid JSON payload")
}

func TestWebhookComplexPayload(t *testing.T) {
	payload := `{
		"link": "https://example.com/complex-article",
		"title": "Complex Article with Multiple Companies",
		"publishDate": "2024-01-01T12:30:00Z",
		"source": "financial-news.com",
		"language": "en",
		"sentiment": "neutral",
		"confidence": "0.85",
		"summary": "A comprehensive analysis of market trends",
		"images": ["https://example.com/image1.jpg", "https://example.com/image2.jpg"],
		"content": "Full article content here...",
		"companies": [
			{"companyId": 1, "confidence": "0.95", "country": "US", "exchange": "NASDAQ",
			 "industry": "Technology", "sector": "Software", "name": "Apple Inc.", "ticker": "AAPL",
			 "isin": "US0378331005", "openfigi": "BBG000B9XRY4"},
			{"companyId": 2, "confidence": "0.88", "name": "Microsoft Corporation", "ticker": "MSFT"}
		]
	}`
	signature := "sha256=" + createSignature(payload, webhookSecret, "")

	article, err := ConstructWebhookEvent([]byte(payload), signature, webhookSecret, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(article.Companies) != 2 {
		t.Fatalf("companies = %d, want 2", len(article.Companies))
	}
	if article.Confidence.Float64() != 0.85 ||
		article.Companies[0].Confidence.Float64() != 0.95 ||
		article.Companies[1].Confidence.Float64() != 0.88 {
		t.Errorf("confidence conversion failed: %+v", article)
	}
	if article.Companies[0].ISIN != "US0378331005" {
		t.Errorf("isin = %q", article.Companies[0].ISIN)
	}
}

func TestWebhookCaseSensitiveSignature(t *testing.T) {
	signature := "sha256=" + strings.ToUpper(createSignature(validWebhookPayload, webhookSecret, ""))

	_, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, "")
	assertVerificationError(t, err, "Invalid webhook signature")
}

func TestWebhookInvalidTimestampFormat(t *testing.T) {
	timestamp := "not-a-timestamp"
	signature := "sha256=" + createSignature(validWebhookPayload, webhookSecret, timestamp)

	_, err := ConstructWebhookEvent([]byte(validWebhookPayload), signature, webhookSecret, timestamp)
	assertVerificationError(t, err, "Invalid timestamp format")
}
