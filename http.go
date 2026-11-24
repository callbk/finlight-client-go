package finlight

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// transport sends REST requests: one X-Request-Id per logical call, retries
// for 5xx, network errors and attempt timeouts, one burst retry after
// Retry-After, never a retry after the caller's cancellation.
type transport struct {
	cfg     Config
	http    *http.Client
	ownHTTP bool
	log     *slog.Logger
	newID   func() string
}

func newTransport(cfg Config) *transport {
	t := &transport{cfg: cfg, http: cfg.HTTPClient, log: cfg.Logger, newID: newRequestID}
	if t.http == nil {
		t.http = &http.Client{}
		t.ownHTTP = true
	}
	return t
}

type apiRequest struct {
	method    string
	path      string     // starting with /v3
	query     url.Values // nil when none
	body      []byte     // nil for GET
	header    http.Header
	requestID string // "" generates one
}

type apiResponse struct {
	status    int
	header    http.Header
	body      []byte
	requestID string
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// do sends one logical request and returns the 2xx (or 304) response.
func (t *transport) do(ctx context.Context, req apiRequest) (*apiResponse, error) {
	if req.requestID == "" {
		req.requestID = t.newID()
	}
	if t.cfg.TotalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, t.cfg.TotalTimeout, errTotalTimeout)
		defer cancel()
	}
	retries := 0
	burstRetried := false
	for {
		resp, err := t.attempt(ctx, req)
		if err != nil {
			if ctx.Err() != nil && !errors.Is(context.Cause(ctx), errTotalTimeout) {
				return nil, ctx.Err() // the caller's cancellation, unchanged
			}
			var te *TimeoutError
			if errors.Is(context.Cause(ctx), errTotalTimeout) && !errors.As(err, &te) {
				err = &TimeoutError{NetworkError: &NetworkError{Message: "total timeout exceeded", Err: err}}
			}
			if retries < t.cfg.MaxRetries && ctx.Err() == nil && isRetryableTransport(err) {
				retries++
				if werr := t.backoff(ctx, retries, req, "network error"); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, err
		}
		if resp.status >= 200 && resp.status < 300 || resp.status == http.StatusNotModified {
			return resp, nil
		}
		apiErr := parseAPIError(resp.status, resp.header, resp.body, req.requestID)
		switch {
		case resp.status == http.StatusTooManyRequests:
			if apiErr.LimitType == RateLimitBurst && t.cfg.RetryOnBurst == BurstRetryOnce && !burstRetried {
				burstRetried = true
				wait := apiErr.RetryAfter
				if wait <= 0 {
					wait = t.cfg.BurstWait
				}
				t.log.Warn("finlight: burst rate limit, retrying once", "wait", wait, "requestId", req.requestID)
				if werr := t.wait(ctx, wait); werr != nil {
					return nil, werr
				}
				continue
			}
			return nil, apiErr
		case resp.status >= 500 && retries < t.cfg.MaxRetries:
			retries++
			if werr := t.backoff(ctx, retries, req, fmt.Sprintf("status %d", resp.status)); werr != nil {
				return nil, werr
			}
			continue
		}
		return nil, apiErr
	}
}

var errTotalTimeout = errors.New("finlight: total timeout exceeded")

func (t *transport) wait(ctx context.Context, d time.Duration) error {
	if err := sleep(ctx, t.cfg.Clock, d); err != nil {
		if errors.Is(context.Cause(ctx), errTotalTimeout) {
			return &TimeoutError{NetworkError: &NetworkError{Message: "total timeout exceeded while waiting to retry", Err: err}}
		}
		return err
	}
	return nil
}

func (t *transport) backoff(ctx context.Context, retry int, req apiRequest, why string) error {
	d := backoffDelay(t.cfg.RetryBaseDelay, t.cfg.RetryMaxDelay, retry-1)
	t.log.Warn("finlight: retrying request", "reason", why, "retry", retry, "delay", d, "requestId", req.requestID)
	return t.wait(ctx, d)
}

// backoffDelay is base*2^n capped at max, with jitter in [50%, 100%].
func backoffDelay(base, maxDelay time.Duration, n int) time.Duration {
	d := float64(base) * math.Pow(2, float64(n))
	if d > float64(maxDelay) {
		d = float64(maxDelay)
	}
	return time.Duration(d * (0.5 + mrand.Float64()*0.5))
}

func isRetryableTransport(err error) bool {
	var ne *NetworkError
	return errors.As(err, &ne)
}

func (t *transport) attempt(ctx context.Context, req apiRequest) (*apiResponse, error) {
	actx := ctx
	if t.ownHTTP {
		var cancel context.CancelFunc
		actx, cancel = context.WithTimeout(ctx, t.cfg.Timeout)
		defer cancel()
	}
	u := t.cfg.BaseURL + req.path
	if len(req.query) > 0 {
		u += "?" + req.query.Encode()
	}
	var body io.Reader
	if req.body != nil {
		body = bytes.NewReader(req.body)
	}
	hr, err := http.NewRequestWithContext(actx, req.method, u, body)
	if err != nil {
		return nil, &ValidationError{Message: "cannot build request: " + err.Error()}
	}
	for k, vs := range req.header {
		hr.Header[k] = vs
	}
	hr.Header.Set("X-API-KEY", t.cfg.APIKey)
	hr.Header.Set("User-Agent", userAgent)
	hr.Header.Set("x-client-version", Version)
	hr.Header.Set("X-Request-Id", req.requestID)
	hr.Header.Set("Accept", "application/json")
	if req.body != nil {
		hr.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.http.Do(hr)
	if err != nil {
		return nil, t.transportError(ctx, actx, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, t.cfg.MaxResponseBytes+1))
	if err != nil {
		return nil, t.transportError(ctx, actx, err)
	}
	if int64(len(data)) > t.cfg.MaxResponseBytes {
		return nil, &DecodeError{Message: fmt.Sprintf("response body exceeds MaxResponseBytes (%d)", t.cfg.MaxResponseBytes)}
	}
	return &apiResponse{status: resp.StatusCode, header: resp.Header, body: data, requestID: req.requestID}, nil
}

// transportError wraps a transport failure; the URL error text is stripped of
// the query so that no request data ends up in error messages.
func (t *transport) transportError(ctx, actx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var nerr net.Error
	if actx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
		return &TimeoutError{NetworkError: &NetworkError{Message: "request timed out", Err: err}}
	}
	return &NetworkError{Message: "request failed", Err: err}
}

// parseAPIError reads an error body: problem+json first, then the NestJS form
// {statusCode, message}, then a non-JSON body as status plus text.
func parseAPIError(status int, header http.Header, body []byte, sentRequestID string) *APIError {
	e := &APIError{Status: status, Raw: string(body), RequestID: sentRequestID}
	var o jsonObject
	trimmed := bytes.TrimSpace(body)
	isObject := len(trimmed) > 0 && trimmed[0] == '{' && json.Unmarshal(trimmed, &o) == nil
	if isObject {
		e.rawJSON = append(json.RawMessage(nil), trimmed...)
	}
	if !isObject {
		e.APIKind = APIKindOther
		e.Detail = strings.TrimSpace(string(body))
		if len(e.Detail) > 500 {
			e.Detail = e.Detail[:500]
		}
	} else if _, hasCode := o["code"]; hasCode || o["type"] != nil || o["detail"] != nil || o["title"] != nil {
		code, _ := o.plainString("", "code")
		e.Code = ProblemCode(code)
		e.Type, _ = o.plainString("", "type")
		e.Title, _ = o.plainString("", "title")
		e.Detail, _ = o.plainString("", "detail")
		if rid, _ := o.plainString("", "requestId"); rid != "" {
			e.RequestID = rid
		}
		if e.Code != "" {
			e.APIKind = apiKindForCode(e.Code)
		} else {
			e.APIKind = APIKindOther
		}
		e.Errors = parseFieldIssues(o["errors"])
		if lt, _ := o.plainString("", "limitType"); lt != "" {
			e.LimitType = RateLimitType(lt)
		}
	} else {
		// NestJS fallback form: {statusCode, message, error}; message may be a
		// string or a list of strings. The status decides the kind.
		e.APIKind = apiKindForStatus(status)
		if msg, ok := o["message"]; ok {
			var s string
			var list []string
			switch {
			case json.Unmarshal(msg, &s) == nil:
				e.Detail = s
			case json.Unmarshal(msg, &list) == nil:
				e.Detail = strings.Join(list, "; ")
				for _, m := range list {
					e.Errors = append(e.Errors, FieldIssue{Issue: m})
				}
			}
		}
		e.Title, _ = o.plainString("", "error")
		if rid, _ := o.plainString("", "requestId"); rid != "" {
			e.RequestID = rid
		}
	}
	if status == http.StatusTooManyRequests {
		e.APIKind = APIKindRateLimited
		if e.LimitType == "" {
			e.LimitType = classifyRateLimit(e.Detail)
		}
		e.RetryAfter = parseRetryAfter(header.Get("Retry-After"))
	}
	return e
}

// classifyRateLimit is the fallback when the 429 body has no limitType.
func classifyRateLimit(detail string) RateLimitType {
	switch {
	case strings.Contains(detail, "Exceeded token limit"):
		return RateLimitMonthly
	case strings.Contains(detail, "Exceeded rate limit"):
		return RateLimitBurst
	}
	return RateLimitUnknown
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
		return time.Duration(f * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func parseFieldIssues(raw json.RawMessage) []FieldIssue {
	if raw == nil || jsonKind(raw) != "array" {
		return nil
	}
	var elems []json.RawMessage
	if json.Unmarshal(raw, &elems) != nil {
		return nil
	}
	out := make([]FieldIssue, 0, len(elems))
	for _, el := range elems {
		o, err := parseObject(el, "")
		if err != nil {
			continue
		}
		fi := FieldIssue{}
		fi.Field, _ = o.plainString("", "field")
		fi.Issue, _ = o.plainString("", "issue")
		if av, err := o.stringList("", "allowedValues"); err == nil && o["allowedValues"] != nil {
			fi.AllowedValues = av
		}
		if c, ok := o["constraints"]; ok {
			var m map[string]string
			if json.Unmarshal(c, &m) == nil {
				fi.Constraints = m
			}
		}
		out = append(out, fi)
	}
	return out
}
