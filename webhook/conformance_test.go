package webhook

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	finlight "github.com/callbk/finlight-client-go/v2"
)

// The W cases of the contract kit run here (the webhook package cannot be
// imported by the root package's tests). Each runs as its own subtest named
// "<id> <title>"; a W case without a test fails.

const kitDir = "../testdata/conformance"

var wCases = map[string]string{
	"W-01": "created-enriched.json",
	"W-02": "updated-with-changed-fields.json",
	"W-03": "updated-content-not-entitled.json",
	"W-04": "created-raw.json",
	"W-05": "test-webhook.json",
	"W-06": "v2-body.json",
	"W-07": "neg-tampered-body.json",
	"W-08": "neg-stale-timestamp.json",
	"W-09": "neg-future-timestamp.json",
	"W-10": "neg-unparseable-timestamp.json",
	"W-11": "neg-unix-seconds-timestamp.json",
	"W-12": "neg-missing-timestamp.json",
	"W-13": "neg-missing-signature.json",
	"W-14": "neg-wrong-prefix.json",
	"W-15": "neg-secret-hex-decoded.json",
	"W-16": "lowercase-headers.json",
}

func TestConformance(t *testing.T) {
	f, err := os.Open(filepath.Join(kitDir, "conformance.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	line := regexp.MustCompile(`^\| (W-\d\d) \| (.+?) \| `)
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		m := line.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		n++
		id, file := m[1], wCases[m[1]]
		t.Run(id+" "+m[2], func(t *testing.T) {
			if file == "" {
				t.Fatalf("kit case %s has no test", id)
			}
			runWebhookCase(t, id, "fixtures/webhook/"+file)
		})
	}
	if n != len(wCases) {
		t.Fatalf("conformance.md lists %d W cases, the runner knows %d", n, len(wCases))
	}
}

type webhookFixture struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
	Verify struct {
		Now              string `json:"now"`
		ToleranceSeconds int    `json:"toleranceSeconds"`
	} `json:"verify"`
	Request struct {
		Headers       map[string]string `json:"headers"`
		RawBodyBase64 string            `json:"rawBodyBase64"`
	} `json:"request"`
	Expect struct {
		Outcome string `json:"outcome"`
		Reason  string `json:"reason"`
		Event   struct {
			EventID        string   `json:"eventId"`
			Event          string   `json:"event"`
			OccurredAt     string   `json:"occurredAt"`
			SubscriptionID string   `json:"subscriptionId"`
			IsTest         bool     `json:"isTest"`
			ChangedFields  []string `json:"changedFields"`
			Link           string   `json:"link"`
		} `json:"event"`
		DetectedVersion       int    `json:"detectedVersion"`
		PayloadVersionPresent bool   `json:"payloadVersionPresent"`
		Link                  string `json:"link"`
	} `json:"expect"`
}

func loadWebhook(t *testing.T, rel string) (webhookFixture, []byte, http.Header, []Option) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(kitDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	var f webhookFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(f.Request.RawBodyBase64)
	if err != nil {
		t.Fatal(err)
	}
	// Header names exactly as the fixture gives them (no canonicalization),
	// so that the lookup has to be case-insensitive.
	h := http.Header{}
	for k, v := range f.Request.Headers {
		h[k] = []string{v}
	}
	now, err := time.Parse(time.RFC3339Nano, f.Verify.Now)
	if err != nil {
		t.Fatal(err)
	}
	opts := []Option{WithNow(func() time.Time { return now }), WithTolerance(time.Duration(f.Verify.ToleranceSeconds) * time.Second)}
	return f, body, h, opts
}

func runWebhookCase(t *testing.T, id, rel string) {
	f, body, h, opts := loadWebhook(t, rel)
	ev, err := ConstructEvent(body, h, f.Secret, opts...)
	switch f.Expect.Outcome {
	case "event":
		if err != nil {
			t.Fatalf("ConstructEvent: %v", err)
		}
		w := f.Expect.Event
		if ev.EventID != w.EventID || string(ev.Event) != w.Event || ev.OccurredAt != w.OccurredAt ||
			ev.SubscriptionID != w.SubscriptionID || ev.IsTest != w.IsTest || ev.Data.Link != w.Link || ev.PayloadVersion != 3 {
			t.Errorf("event = %+v", ev)
		}
		if !slices.Equal(ev.ChangedFields, w.ChangedFields) {
			t.Errorf("changedFields = %v, want %v", ev.ChangedFields, w.ChangedFields)
		}
		if w.ChangedFields == nil && ev.ChangedFields != nil {
			t.Error("absent changedFields must read as nil")
		}
		if string(ev.RawJSON()) != string(body) {
			t.Error("RawJSON differs from the body")
		}
		// The same body as a string (converted to bytes) verifies too.
		if _, err := ConstructEvent([]byte(string(body)), h, f.Secret, opts...); err != nil {
			t.Errorf("string body: %v", err)
		}
		switch id {
		case "W-03":
			if ev.Data.ContentPresent() {
				t.Error("content must be absent without fullArticleAccess")
			}
		case "W-04":
			a := ev.Data
			if a.ContentPresent() || a.SentimentPresent() || a.CompaniesPresent() || a.EventsPresent() || a.AvailableAt != nil {
				t.Errorf("raw data = %+v", a)
			}
			if a.FirstSeenAt == nil || ev.OccurredAt != *a.FirstSeenAt {
				t.Error("occurredAt must equal data.firstSeenAt on raw")
			}
		case "W-05":
			if !strings.HasPrefix(ev.EventID, "evt_test") || !ev.Data.ContentPresent() {
				t.Error("test webhook")
			}
		case "W-01", "W-16":
			if !ev.Data.CompaniesPresent() || !strings.Contains(ev.Data.Title, "Ümsatz") {
				t.Error("non-ASCII body not decoded")
			}
			if tm, ok := ev.OccurredAtTime(); !ok || tm.IsZero() {
				t.Error("occurredAt does not parse")
			}
		}
	case "wrongPayloadVersion":
		var w *WrongPayloadVersionError
		if !errors.As(err, &w) {
			t.Fatalf("err = %v, want *WrongPayloadVersionError", err)
		}
		var ve *VerificationError
		var de *finlight.DecodeError
		if errors.As(err, &ve) || errors.As(err, &de) || ev != nil {
			t.Fatal("a wrong payload version is neither a verification nor a decode error")
		}
		if w.DetectedVersion != f.Expect.DetectedVersion || w.PayloadVersionPresent != f.Expect.PayloadVersionPresent ||
			w.PayloadVersion != nil || w.Link != f.Expect.Link || w.Raw["link"] != f.Expect.Link {
			t.Errorf("wrong payload version = %+v", w)
		}
		if string(w.RawJSON()) != string(body) {
			t.Error("RawJSON differs from the body")
		}
	case "error":
		var ve *VerificationError
		if !errors.As(err, &ve) || ev != nil {
			t.Fatalf("err = %v, want *VerificationError", err)
		}
		if string(ve.Reason) != f.Expect.Reason || ve.Kind() != finlight.ErrorKindWebhookVerification {
			t.Errorf("reason = %s, want %s", ve.Reason, f.Expect.Reason)
		}
		if strings.Contains(err.Error(), f.Secret) {
			t.Error("the secret leaked into the error")
		}
	default:
		t.Fatalf("unknown outcome %s", f.Expect.Outcome)
	}
}
