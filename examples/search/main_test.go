package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func kitResponse(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/conformance/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Response struct {
			Body json.RawMessage `json:"body"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f.Response.Body
}

func TestSearchExample(t *testing.T) {
	body := kitResponse(t, "fixtures/requests/search-all-parameters.json")
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := run(t.Context(), &out, finlight.Config{APIKey: "k", BaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sent), `"query":"ticker:NVDA"`) || !strings.Contains(out.String(), "hasMore:") || !strings.Contains(out.String(), "Synthetic page item 1") {
		t.Errorf("sent %s\noutput:\n%s", sent, out.String())
	}

	// An API error is printed with status, code and request id.
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","status":401,"detail":"Unauthorized","requestId":"req-1"}`))
	}))
	defer errSrv.Close()
	out.Reset()
	if err := run(t.Context(), &out, finlight.Config{APIKey: "k", BaseURL: errSrv.URL}); err == nil || !strings.Contains(out.String(), "status 401, code UNAUTHORIZED, request req-1") {
		t.Errorf("output = %s", out.String())
	}
}
