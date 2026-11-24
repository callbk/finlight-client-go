// Command webhook-endpoint receives finlight webhooks.
//
//	FINLIGHT_WEBHOOK_SECRET=... go run ./examples/webhook-endpoint
package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	finlight "github.com/callbk/finlight-client-go/v2"
	"github.com/callbk/finlight-client-go/v2/webhook"
)

func main() {
	secret := os.Getenv("FINLIGHT_WEBHOOK_SECRET")
	work := make(chan *finlight.WebhookEvent, 100)
	go func() {
		for ev := range work {
			// Slow work happens here, after the 2xx answer.
			log.Printf("%s %s: %s", ev.Event, ev.EventID, ev.Data.Title)
		}
	}()
	log.Fatal(http.ListenAndServe(":8080", newHandler(secret, work)))
}

// newHandler answers within 5 s: 2xx for accepted, duplicate and old-format
// deliveries, 400 for a failed verification, 503 when the work queue is full
// (finlight retries the same eventId later). Never 410: it disables the webhook.
func newHandler(secret string, work chan<- *finlight.WebhookEvent) http.Handler {
	var seen sync.Map // eventId -> struct{}; use a shared store in production
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The raw bytes exactly as received; never re-serialize parsed JSON.
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		ev, err := webhook.ConstructEvent(body, r.Header, secret)
		var wrong *webhook.WrongPayloadVersionError
		var verr *webhook.VerificationError
		switch {
		case errors.As(err, &wrong):
			// An old-format retry from before the switch to payload version 3:
			// keep the raw JSON and answer 2xx.
			log.Printf("old payload version for %s kept (%d bytes)", wrong.Link, len(wrong.RawJSON()))
			w.WriteHeader(http.StatusOK)
			return
		case errors.As(err, &verr):
			http.Error(w, "verification failed", http.StatusBadRequest)
			return
		case err != nil:
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		if _, dup := seen.LoadOrStore(ev.EventID, struct{}{}); dup {
			w.WriteHeader(http.StatusOK) // a retry of a delivery we already have
			return
		}
		// Queue first, answer after: a 2xx ends the deliveries of this event.
		select {
		case work <- ev:
			w.WriteHeader(http.StatusOK)
		default:
			// Not taken: forget the eventId and answer 503, so that the retry
			// (same eventId, 10 min later) is not dropped as a duplicate.
			seen.Delete(ev.EventID)
			log.Printf("work queue full, asking for a retry of %s", ev.EventID)
			http.Error(w, "busy, retry later", http.StatusServiceUnavailable)
		}
	})
}
