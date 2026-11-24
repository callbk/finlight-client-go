package finlight

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A 3xx is not followed: the key never reaches the redirect target.
func TestRedirectNotFollowed(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "" {
			leaked.Add(1)
		}
		_, _ = io.WriteString(w, `{"sources":[]}`)
	}))
	defer target.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer api.Close()
	for _, hc := range []*http.Client{nil, {}} {
		c := newTestClient(t, Config{BaseURL: api.URL, HTTPClient: hc})
		_, err := c.Sources.List(t.Context())
		var ae *APIError
		if !errors.As(err, &ae) || ae.Status != http.StatusFound || ae.APIKind != APIKindOther {
			t.Errorf("err = %v, want an *APIError with status 302", err)
		}
	}
	if leaked.Load() != 0 {
		t.Error("the API key reached the redirect target")
	}
}

// R-RETRY-1: net/http does not replay a GET on its own when a reused
// keep-alive connection breaks after the request was written.
func TestGetNotReplayedByNetHTTP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var requests atomic.Int32
	go func() {
		first := true
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn, first bool) {
				defer conn.Close()
				br := bufio.NewReader(conn)
				for n := 0; ; n++ {
					req, err := http.ReadRequest(br)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					requests.Add(1)
					if first && n == 1 {
						// The second request on the reused connection: drop it
						// after it was written, before any response byte.
						return
					}
					_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 14\r\n\r\n"+`{"sources":[]}`)
				}
			}(conn, first)
			first = false
		}
	}()
	c := newTestClient(t, Config{BaseURL: "http://" + ln.Addr().String(), MaxRetries: -1})
	if _, err := c.Sources.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = c.Sources.List(t.Context())
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Errorf("err = %v, want a *NetworkError", err)
	}
	if n := requests.Load(); n != 2 {
		t.Errorf("the server saw %d requests, want 2 (no hidden replay)", n)
	}
}

// A GET goes out without a body and without Content-Length or chunking.
func TestGetSendsNoBody(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	head := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		var sb strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			sb.WriteString(line)
			if line == "\r\n" {
				break
			}
		}
		head <- sb.String()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 14\r\nConnection: close\r\n\r\n"+`{"sources":[]}`)
	}()
	c := newTestClient(t, Config{BaseURL: "http://" + ln.Addr().String()})
	if _, err := c.Sources.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	h := strings.ToLower(<-head)
	if strings.Contains(h, "content-length") || strings.Contains(h, "transfer-encoding") {
		t.Errorf("GET carries a body header:\n%s", h)
	}
}

// Contract section 6: the list envelope is always there. A 200 without it is
// a *DecodeError, not an empty result.
func TestMissingEnvelopeIsDecodeError(t *testing.T) {
	for _, body := range []map[string]any{{}, {"articles": nil, "companies": nil, "sources": nil}} {
		m := newHTTPMock(t)
		m.setAlways(mockResponse{Status: 200, Body: body})
		c := newTestClient(t, Config{BaseURL: m.URL()})
		ctx := t.Context()
		calls := map[string]func() error{
			"articles": func() error { _, err := c.Articles.Search(ctx, SearchParams{}); return err },
			"companies": func() error {
				_, err := c.Companies.Search(ctx, CompanySearchParams{Q: "x"})
				return err
			},
			"sources": func() error { _, err := c.Sources.List(ctx); return err },
		}
		for key, call := range calls {
			var de *DecodeError
			if err := call(); !errors.As(err, &de) || de.Path != key {
				t.Errorf("%s with body %v: err = %v, want a *DecodeError at %s", key, body, err, key)
			}
		}
	}
}
