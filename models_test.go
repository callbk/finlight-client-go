package finlight

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexFloatUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{`"0.95"`, 0.95},
		{`0.95`, 0.95},
		{`"1"`, 1},
		{`null`, 0},
	}
	for _, tc := range cases {
		var f FlexFloat
		if err := json.Unmarshal([]byte(tc.in), &f); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.in, err)
		}
		if f.Float64() != tc.want {
			t.Errorf("unmarshal %s: got %v, want %v", tc.in, f.Float64(), tc.want)
		}
	}

	var f FlexFloat
	if err := json.Unmarshal([]byte(`"abc"`), &f); err == nil {
		t.Error("expected error for non-numeric string")
	}
}

func TestFlexTimeUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{`"2024-01-01T00:00:00Z"`, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{`"2024-01-01T12:30:00.123456+00:00"`, time.Date(2024, 1, 1, 12, 30, 0, 123456000, time.UTC)},
		{`"2024-01-01T12:30:00"`, time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)},
		{`"2024-01-01 12:30:00"`, time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)},
		{`"2024-01-01"`, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		var ft FlexTime
		if err := json.Unmarshal([]byte(tc.in), &ft); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.in, err)
		}
		if !ft.Time.Equal(tc.want) {
			t.Errorf("unmarshal %s: got %v, want %v", tc.in, ft.Time, tc.want)
		}
	}

	var ft FlexTime
	if err := json.Unmarshal([]byte(`"not a date"`), &ft); err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestGetArticlesParamsOmitsZeroFields(t *testing.T) {
	data, err := json.Marshal(GetArticlesParams{Query: "nvidia"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"query":"nvidia"}` {
		t.Errorf("unexpected serialization: %s", data)
	}

	data, err = json.Marshal(GetArticlesParams{From: "2024-01-01", Categories: []Category{CategoryMarkets}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"from":"2024-01-01","categories":["markets"]}` {
		t.Errorf("unexpected serialization: %s", data)
	}
}

func TestArticleDecodesStringConfidence(t *testing.T) {
	raw := `{
		"link": "https://example.com/a",
		"title": "T",
		"publishDate": "2024-01-01T00:00:00Z",
		"source": "example.com",
		"language": "en",
		"confidence": "0.95",
		"companies": [{"companyId": 1, "name": "Apple Inc.", "ticker": "AAPL", "confidence": "0.90"}]
	}`
	var a Article
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	if a.Confidence == nil || a.Confidence.Float64() != 0.95 {
		t.Errorf("article confidence: got %v, want 0.95", a.Confidence)
	}
	if len(a.Companies) != 1 || a.Companies[0].Confidence == nil || a.Companies[0].Confidence.Float64() != 0.90 {
		t.Errorf("company confidence not decoded: %+v", a.Companies)
	}
}
