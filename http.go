package finlight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// retryableStatus mirrors the sibling clients: retry on 429 and transient 5xx.
var retryableStatus = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

const baseRetryDelay = 500 * time.Millisecond

// apiClient performs authenticated REST requests with retry and backoff.
type apiClient struct {
	cfg  Config
	http *http.Client
	log  *slog.Logger
}

func newAPIClient(cfg Config) *apiClient {
	return &apiClient{cfg: cfg, http: cfg.HTTPClient, log: cfg.Logger}
}

// do sends one API request, retrying retryable statuses with exponential
// backoff (500ms · 2^(attempt−1)), and decodes the 2xx response into out.
func (c *apiClient) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.cfg.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("finlight: encode request: %w", err)
		}
	}

	for attempt := 1; ; attempt++ {
		respBody, apiErr, err := c.doOnce(ctx, method, u, payload)
		if err != nil {
			return err
		}
		if apiErr != nil {
			if retryableStatus[apiErr.StatusCode] && attempt < c.cfg.RetryCount {
				delay := baseRetryDelay << (attempt - 1)
				c.log.Warn("finlight: retrying request", "status", apiErr.StatusCode, "attempt", attempt, "delay", delay)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return apiErr
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("finlight: decode response: %w", err)
		}
		return nil
	}
}

func (c *apiClient) doOnce(ctx context.Context, method, url string, payload []byte) ([]byte, *APIError, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("finlight: build request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.cfg.APIKey)
	req.Header.Set("User-Agent", clientVersion())
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("finlight: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("finlight: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: respBody}, nil
	}
	return respBody, nil, nil
}
