package finlight

import (
	"context"
	"iter"
)

// RawWebSocketClient streams unenriched articles in real time (no sentiment,
// entities, or content — lower latency). No duplicate suppression.
//
// A client instance supports one active stream at a time.
type RawWebSocketClient struct {
	core *wsCore[RawArticle]
}

// NewRawWebSocketClient returns a raw streaming client with custom options.
// Client instances created by New use default options.
func NewRawWebSocketClient(cfg Config, opts WebSocketOptions) *RawWebSocketClient {
	cfg = cfg.withDefaults()
	return &RawWebSocketClient{
		core: newWSCore[RawArticle](cfg, opts, cfg.WssURL+"/raw", nil),
	}
}

// Stream connects to the raw finlight WebSocket and yields articles matching
// params. See WebSocketClient.Stream for the streaming semantics.
func (w *RawWebSocketClient) Stream(ctx context.Context, params GetRawArticlesWebSocketParams) iter.Seq[RawArticle] {
	return w.core.stream(ctx, params)
}

// Err reports why the last stream ended: nil after a normal shutdown, or a
// terminal error such as ErrBlocked.
func (w *RawWebSocketClient) Err() error {
	return w.core.getErr()
}
