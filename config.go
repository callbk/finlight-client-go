package finlight

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Defaults of every Config key (see the README table).
const (
	defaultBaseURL                = "https://api.finlight.me"
	defaultWSSURL                 = "wss://wss.finlight.me"
	defaultTimeout                = 30 * time.Second
	defaultMaxRetries             = 3
	defaultRetryBaseDelay         = 500 * time.Millisecond
	defaultRetryMaxDelay          = 8 * time.Second
	defaultBurstWait              = 10 * time.Second
	defaultMaxPages               = 10
	defaultMaxResponseBytes       = 32 << 20
	defaultPingInterval           = 25 * time.Second
	defaultWatchdogTimeout        = 60 * time.Second
	defaultRotationInterval       = 115 * time.Minute
	defaultReconnectBaseDelay     = 1 * time.Second
	defaultReconnectMaxDelay      = 30 * time.Second
	defaultLongBackoff            = 60 * time.Second
	defaultQuotaReconnectInterval = time.Hour
	defaultSearchFailedThreshold  = 5
	defaultDedupeCacheSize        = 1000
	defaultMaxMessageBytes        = 32 << 20
	defaultConnectTimeout         = 30 * time.Second
)

// BurstRetry says what the SDK does on a 429 with limitType "burst".
type BurstRetry string

// Burst retry policies. The zero value ("") means BurstRetryOnce.
const (
	// BurstRetryOnce waits exactly Retry-After (BurstWait when the header is
	// missing) and retries once. It does not count against MaxRetries.
	BurstRetryOnce BurstRetry = "once"
	// BurstRetryOff returns the RateLimited error at once.
	BurstRetryOff BurstRetry = "off"
)

// Config configures a Client. Only APIKey is required; every zero-valued field
// falls back to its documented default. Integer limits that may be turned off
// (MaxRetries) take a negative value for "off", because 0 means "default" in
// Go.
type Config struct {
	APIKey  string // required
	BaseURL string // default https://api.finlight.me
	WSSURL  string // default wss://wss.finlight.me (raw streams use WSSURL + "/raw")

	// Timeout bounds one REST attempt (default 30 s). It is applied only when
	// the SDK owns the HTTP client; an injected HTTPClient keeps its own timeouts.
	Timeout time.Duration
	// TotalTimeout bounds a whole call including retries and burst waits.
	// Zero means no total limit.
	TotalTimeout time.Duration
	// MaxRetries is the number of retries after the first attempt for 5xx,
	// network errors and attempt timeouts (default 3, negative: none).
	MaxRetries     int
	RetryBaseDelay time.Duration // first backoff, default 0.5 s
	RetryMaxDelay  time.Duration // backoff cap, default 8 s
	// RetryOnBurst controls the single retry after a burst 429 ("" = once).
	RetryOnBurst BurstRetry
	// BurstWait is waited before the burst retry when Retry-After is missing
	// (default 10 s).
	BurstWait time.Duration
	// MaxPages caps Articles.SearchAll (default 10).
	MaxPages int
	// MaxResponseBytes caps a REST response body (default 32 MiB).
	MaxResponseBytes int64

	PingInterval       time.Duration // default 25 s
	WatchdogTimeout    time.Duration // default 60 s without pong
	RotationInterval   time.Duration // default 115 min
	ReconnectBaseDelay time.Duration // default 1 s
	ReconnectMaxDelay  time.Duration // default 30 s
	LongBackoff        time.Duration // default 60 s (connection limit, dial 429)
	// DisableQuotaReconnect ends a stream with its error when the WebSocket
	// quota is used up. By default the stream redials every
	// QuotaReconnectInterval until the quota allows it.
	DisableQuotaReconnect  bool
	QuotaReconnectInterval time.Duration // default 1 h
	SearchFailedThreshold  int           // SEARCH_FAILED in a row before OnError, default 5
	DedupeCacheSize        int           // default 1000
	MaxMessageBytes        int64         // default 32 MiB, at least 16 MiB
	ConnectTimeout         time.Duration // WebSocket handshake, default 30 s

	// Logger receives the SDK's log records. Default: silent.
	Logger *slog.Logger
	// Clock replaces the system clock (for tests). Default: the system clock.
	Clock Clock
	// HTTPClient is used for REST and for the WebSocket handshake. The SDK
	// never closes it. Its own timeouts apply. REST uses a shallow copy with
	// redirects turned off, so that the key never goes to another host.
	HTTPClient *http.Client
}

// String hides the API key so that printing a Config never leaks it.
func (c Config) String() string {
	return fmt.Sprintf("finlight.Config{BaseURL:%q, WSSURL:%q, APIKey:%s}", c.BaseURL, c.WSSURL, redact(c.APIKey))
}

// GoString hides the API key in %#v output.
func (c Config) GoString() string { return c.String() }

// Format prints String for every verb (also %d or %x), so that no verb
// reaches the APIKey field.
func (c Config) Format(f fmt.State, verb rune) { formatRedacted(f, verb, c.String()) }

func redact(s string) string {
	if s == "" {
		return `""`
	}
	return "[redacted]"
}

func (c Config) withDefaults() (Config, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return c, validationErr("APIKey", "APIKey is required and must not be empty")
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.WSSURL == "" {
		c.WSSURL = defaultWSSURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	c.WSSURL = strings.TrimRight(c.WSSURL, "/")
	setDur := func(d *time.Duration, def time.Duration) {
		if *d <= 0 {
			*d = def
		}
	}
	setDur(&c.Timeout, defaultTimeout)
	setDur(&c.RetryBaseDelay, defaultRetryBaseDelay)
	setDur(&c.RetryMaxDelay, defaultRetryMaxDelay)
	setDur(&c.BurstWait, defaultBurstWait)
	setDur(&c.PingInterval, defaultPingInterval)
	setDur(&c.WatchdogTimeout, defaultWatchdogTimeout)
	setDur(&c.RotationInterval, defaultRotationInterval)
	setDur(&c.ReconnectBaseDelay, defaultReconnectBaseDelay)
	setDur(&c.ReconnectMaxDelay, defaultReconnectMaxDelay)
	setDur(&c.LongBackoff, defaultLongBackoff)
	setDur(&c.QuotaReconnectInterval, defaultQuotaReconnectInterval)
	setDur(&c.ConnectTimeout, defaultConnectTimeout)
	if c.TotalTimeout < 0 {
		c.TotalTimeout = 0
	}
	switch {
	case c.MaxRetries == 0:
		c.MaxRetries = defaultMaxRetries
	case c.MaxRetries < 0:
		c.MaxRetries = 0
	}
	switch c.RetryOnBurst {
	case "":
		c.RetryOnBurst = BurstRetryOnce
	case BurstRetryOnce, BurstRetryOff:
	default:
		return c, validationErr("RetryOnBurst", "RetryOnBurst must be %q or %q", BurstRetryOnce, BurstRetryOff)
	}
	if c.MaxPages <= 0 {
		c.MaxPages = defaultMaxPages
	}
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = defaultMaxResponseBytes
	}
	if c.SearchFailedThreshold <= 0 {
		c.SearchFailedThreshold = defaultSearchFailedThreshold
	}
	if c.DedupeCacheSize <= 0 {
		c.DedupeCacheSize = defaultDedupeCacheSize
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultMaxMessageBytes
	}
	if c.MaxMessageBytes < 16<<20 {
		c.MaxMessageBytes = 16 << 20
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	c.Logger = c.Logger.With(slog.String("logger", "finlight"))
	if c.Clock == nil {
		c.Clock = systemClock{}
	}
	return c, nil
}
