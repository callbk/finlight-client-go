package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	finlight "github.com/callbk/finlight-client-go/v2"
	"github.com/coder/websocket"
)

func TestStreamExample(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := context.Background()
		_, sub, err := c.Read(ctx)
		if err != nil {
			return
		}
		var m map[string]any
		_ = json.Unmarshal(sub, &m)
		if m["payloadVersion"] != float64(3) || m["includeUpdates"] != true {
			_ = c.Close(websocket.StatusPolicyViolation, "bad subscribe")
			return
		}
		send := func(s string) { _ = c.Write(ctx, websocket.MessageText, []byte(s)) }
		send(`{"event":"connection.admitted","payloadVersion":3,"data":{"leaseId":"lease-1","serverNow":1,"clientNonce":null}}`)
		send(`{"event":"connection.draining","payloadVersion":3,"data":{}}`)
		for i := 0; i < 6; i++ {
			send(fmt.Sprintf(`{"event":"article.created","payloadVersion":3,"data":{"id":"a%d","link":"https://x.example/%d","title":"Title %d","source":{"domain":"x.example","type":null}}}`, i, i, i))
		}
		for {
			if _, _, err := c.Read(ctx); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	var out bytes.Buffer
	err := run(t.Context(), &out, finlight.Config{APIKey: "k", WSSURL: "ws" + strings.TrimPrefix(srv.URL, "http")}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "admitted, lease lease-1") || !strings.Contains(out.String(), "created: Title 4") || strings.Contains(out.String(), "Title 5") {
		t.Errorf("output:\n%s", out.String())
	}
}
