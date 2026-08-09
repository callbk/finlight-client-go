// Package finlight is the official Go client for the finlight.me API.
//
// It provides access to the REST API (article search, article lookup by
// link, source listing), real-time article streaming over WebSocket, and
// webhook signature verification.
//
// # Quick start
//
//	client, err := finlight.New(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
//	if err != nil {
//		log.Fatal(err)
//	}
//	resp, err := client.Articles.FetchArticles(ctx, finlight.GetArticlesParams{Query: "nvidia"})
//
// # Streaming
//
// WebSocket streaming uses a range-over-func iterator. The internal
// reconnect loop (exponential backoff, proactive connection rotation,
// rate-limit handling) is transparent to the consumer:
//
//	for article := range client.Websocket.Stream(ctx, finlight.GetArticlesWebSocketParams{}) {
//		fmt.Println(article.Title)
//	}
//	if err := client.Websocket.Err(); err != nil {
//		log.Fatal(err)
//	}
//
// Stop streaming by breaking out of the loop or cancelling the context.
package finlight
