package finlight_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"

	finlight "github.com/callbk/finlight-client-go"
)

func ExampleNew() {
	client, err := finlight.New(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Articles.FetchArticles(context.Background(), finlight.GetArticlesParams{
		Query:      "nvidia",
		PageSize:   10,
		Categories: []finlight.Category{finlight.CategoryTechnology},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, article := range resp.Articles {
		fmt.Println(article.Title)
	}
}

func ExampleWebSocketClient_Stream() {
	client, err := finlight.New(finlight.Config{APIKey: os.Getenv("FINLIGHT_API_KEY")})
	if err != nil {
		log.Fatal(err)
	}

	// Stop streaming on Ctrl+C; breaking out of the loop works too.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for article := range client.Websocket.Stream(ctx, finlight.GetArticlesWebSocketParams{
		Tickers: []string{"AAPL", "NVDA"},
	}) {
		fmt.Println(article.Title)
	}
	if err := client.Websocket.Err(); err != nil {
		log.Fatal(err)
	}
}

func ExampleConstructWebhookEvent() {
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		article, err := finlight.ConstructWebhookEvent(
			body,
			r.Header.Get("X-Webhook-Signature"),
			os.Getenv("FINLIGHT_WEBHOOK_SECRET"),
			r.Header.Get("X-Webhook-Timestamp"),
		)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Println("new article:", article.Title)
		w.WriteHeader(http.StatusOK)
	})
}
