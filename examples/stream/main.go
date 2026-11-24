// Command stream prints the first five articles of a live stream.
//
//	FINLIGHT_API_KEY=... go run ./examples/stream
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Stdout, configFromEnv(), 5); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func configFromEnv() finlight.Config {
	return finlight.Config{
		APIKey:  os.Getenv("FINLIGHT_API_KEY"),
		BaseURL: os.Getenv("FINLIGHT_BASE_URL"),
		WSSURL:  os.Getenv("FINLIGHT_WSS_URL"),
	}
}

func run(ctx context.Context, w io.Writer, cfg finlight.Config, max int) error {
	client, err := finlight.NewClient(cfg)
	if err != nil {
		return err
	}
	includeUpdates := true
	s, err := client.Stream(ctx, finlight.StreamParams{
		Query:          "ticker:NVDA",
		Include:        []finlight.IncludeBlock{finlight.IncludeBlockCompanies},
		IncludeUpdates: &includeUpdates,
	}, &finlight.StreamOptions{
		// Admitted comes before the subscribe is validated.
		OnAdmitted: func(a finlight.AdmittedInfo) { fmt.Fprintf(w, "admitted, lease %s\n", a.LeaseID) },
		// No automatic backfill: use LastSeenAt with Articles.Search if needed.
		OnReconnect: func(r finlight.ReconnectInfo) {
			last := "nothing yet"
			if r.LastSeenAt != nil {
				last = *r.LastSeenAt
			}
			fmt.Fprintf(w, "reconnected (%s), last seen %s\n", r.Reason, last)
		},
		OnError: func(e finlight.Error) { fmt.Fprintf(w, "stream problem (%s): %v\n", e.Kind(), e) },
	})
	if err != nil {
		return err
	}
	n := 0
	for ev, err := range s.Events() {
		if err != nil {
			var pre *finlight.PreemptedError
			var apiErr *finlight.APIError
			switch {
			case errors.As(err, &pre):
				fmt.Fprintf(w, "preempted: %s\n", pre.Reason)
			case errors.As(err, &apiErr):
				fmt.Fprintf(w, "refused: status %d, %s\n", apiErr.Status, apiErr.Detail)
			}
			return err
		}
		if ev.Kind == finlight.StreamEventKindOther {
			continue // an event this SDK version does not know
		}
		fmt.Fprintf(w, "%s: %s\n", ev.Kind, ev.Data.Title)
		if n++; n == max {
			break // closes the stream
		}
	}
	return nil
}
