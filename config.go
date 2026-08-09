package finlight

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Config configures the finlight client. Only APIKey is required; zero-valued
// fields fall back to the documented defaults.
type Config struct {
	APIKey     string        // required
	BaseURL    string        // default "https://api.finlight.me"
	WssURL     string        // default "wss://wss.finlight.me"
	Timeout    time.Duration // per-request timeout, default 5s
	RetryCount int           // total request attempts, default 3
	Logger     *slog.Logger  // default: silent
	HTTPClient *http.Client  // optional override; default uses Timeout
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.finlight.me"
	}
	if c.WssURL == "" {
		c.WssURL = "wss://wss.finlight.me"
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.RetryCount == 0 {
		c.RetryCount = 3
	}
	if c.Logger == nil {
		c.Logger = slog.New(discardHandler{})
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: c.Timeout}
	}
	return c
}

// discardHandler is a slog.Handler that drops everything. (slog.DiscardHandler
// exists from Go 1.24; this keeps the module compatible with 1.23.)
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }
