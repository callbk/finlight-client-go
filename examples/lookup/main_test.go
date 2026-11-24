package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func kitResponse(t *testing.T, rel string) (json.RawMessage, map[string]string) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/conformance/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Response struct {
			Headers map[string]string `json:"headers"`
			Body    json.RawMessage   `json:"body"`
		} `json:"response"`
	}
	_ = json.Unmarshal(data, &f)
	return f.Response.Body, f.Response.Headers
}

func TestLookupExample(t *testing.T) {
	article, _ := kitResponse(t, "fixtures/requests/get-by-link.json")
	notFound, _ := kitResponse(t, "fixtures/errors/not-found-id.json")
	companies, _ := kitResponse(t, "fixtures/requests/companies-search.json")
	company, hdr := kitResponse(t, "fixtures/requests/companies-get.json")
	sources, _ := kitResponse(t, "fixtures/requests/sources-list-full.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/articles/by-link":
			_, _ = w.Write(article)
		case strings.HasPrefix(r.URL.Path, "/v3/articles/"):
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(404)
			_, _ = w.Write(notFound)
		case r.URL.Path == "/v3/companies":
			_, _ = w.Write(companies)
		case strings.HasPrefix(r.URL.Path, "/v3/companies/"):
			w.Header().Set("ETag", hdr["etag"])
			w.Header().Set("Cache-Control", hdr["cache-control"])
			_, _ = w.Write(company)
		case r.URL.Path == "/v3/sources":
			_, _ = w.Write(sources)
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := run(t.Context(), &out, finlight.Config{APIKey: "k", BaseURL: srv.URL}, "https://news.example.de/wirtschaft/beispiel-werke-rekordquartal"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"by link:", "company 900001", "etag", "3 sources"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
