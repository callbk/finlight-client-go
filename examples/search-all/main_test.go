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

func TestSearchAllExample(t *testing.T) {
	data, err := os.ReadFile("../../testdata/conformance/fixtures/pagination/three-pages.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Steps []struct {
			Response struct {
				Body json.RawMessage `json:"body"`
			} `json:"response"`
		} `json:"steps"`
	}
	_ = json.Unmarshal(data, &f)
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve pages 1 and 2 again and again: the walk stops at maxPages 3.
		_, _ = w.Write(f.Steps[n%2].Response.Body)
		n++
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := run(t.Context(), &out, finlight.Config{APIKey: "k", BaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "6 articles on 3 pages") || !strings.Contains(out.String(), "stopped at maxPages") {
		t.Errorf("output:\n%s", out.String())
	}
}
