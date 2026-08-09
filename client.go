package finlight

import "errors"

// Client is the entry point to the finlight API.
type Client struct {
	Articles     *ArticleService
	Sources      *SourceService
	Websocket    *WebSocketClient    // enhanced article stream, default options
	RawWebsocket *RawWebSocketClient // raw article stream, default options
}

// New validates cfg, applies defaults, and returns a ready-to-use client.
// For WebSocket clients with custom options use NewWebSocketClient or
// NewRawWebSocketClient.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("finlight: Config.APIKey is required")
	}
	cfg = cfg.withDefaults()
	api := newAPIClient(cfg)
	return &Client{
		Articles:     &ArticleService{api: api},
		Sources:      &SourceService{api: api},
		Websocket:    NewWebSocketClient(cfg, WebSocketOptions{}),
		RawWebsocket: NewRawWebSocketClient(cfg, WebSocketOptions{}),
	}, nil
}
