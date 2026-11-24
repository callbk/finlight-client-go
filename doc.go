// Package finlight is the official Go client for the finlight API v3: article
// search and lookup, company master data, sources, real-time article streams
// over WebSocket, and (in the webhook subpackage) webhook verification.
//
// Version 2 of this module (github.com/callbk/finlight-client-go/v2) speaks
// API v3 only. Version 1 (the module path without /v2) speaks API v2.
//
// # Quick start
//
//	client, err := finlight.NewClient(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
//	if err != nil {
//		log.Fatal(err)
//	}
//	page, err := client.Articles.Search(ctx, finlight.SearchParams{Query: "ticker:NVDA", Limit: 10})
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, a := range page.Articles {
//		fmt.Println(a.Title)
//	}
//
// # Absent, null and empty
//
// The gated keys of an article (Content, Sentiment, Companies, Events) are
// Field values with three states: absent (not entitled, not requested, or a
// raw article), null (entitled, but no value) and a value. Use
// Article.ContentPresent and its siblings, or Field.Present, Field.IsNull and
// Field.Get. An empty Companies list means "tagged, no company"; an absent one
// means "not delivered".
//
// # Streams
//
//	s, err := client.Stream(ctx, finlight.StreamParams{Query: "ticker:NVDA"}, nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for ev, err := range s.Events() {
//		if err != nil {
//			log.Fatal(err) // terminal: refused dial, VALIDATION_FAILED, preemption
//		}
//		if ev.Kind != finlight.StreamEventKindOther {
//			fmt.Println(ev.Data.Title)
//		}
//	}
//
// Stop a stream by cancelling its context, breaking out of the loop or
// calling Close.
package finlight
