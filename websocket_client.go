package finlight

import (
	"context"
	"iter"
)

// WebSocketClient streams enriched articles in real time. Duplicate articles
// (same link within the last 10 deliveries) are suppressed.
//
// A client instance supports one active stream at a time.
type WebSocketClient struct {
	core *wsCore[Article]
}

// NewWebSocketClient returns a streaming client with custom options.
// Client instances created by New use default options.
func NewWebSocketClient(cfg Config, opts WebSocketOptions) *WebSocketClient {
	cfg = cfg.withDefaults()
	return &WebSocketClient{
		core: newWSCore(cfg, opts, cfg.WssURL, func(a Article) string { return a.Link }),
	}
}

// Stream connects to the finlight WebSocket and yields articles matching
// params. Reconnects (exponential backoff, proactive rotation, rate-limit
// waits) are handled internally. End the stream by breaking out of the range
// loop or cancelling ctx; afterwards check Err for a terminal failure.
func (w *WebSocketClient) Stream(ctx context.Context, params GetArticlesWebSocketParams) iter.Seq[Article] {
	return w.core.stream(ctx, params)
}

// Err reports why the last stream ended: nil after a normal shutdown (context
// cancelled or consumer break), or a terminal error such as ErrBlocked.
func (w *WebSocketClient) Err() error {
	return w.core.getErr()
}
