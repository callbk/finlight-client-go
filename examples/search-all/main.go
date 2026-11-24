// Command search-all walks up to three pages of a search.
//
//	FINLIGHT_API_KEY=... go run ./examples/search-all
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	finlight "github.com/callbk/finlight-client-go/v2"
)

func main() {
	if err := run(context.Background(), os.Stdout, configFromEnv()); err != nil {
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

func run(ctx context.Context, w io.Writer, cfg finlight.Config) error {
	client, err := finlight.NewClient(cfg)
	if err != nil {
		return err
	}
	// Every page costs quota; MaxPages caps the walk.
	res, err := client.Articles.SearchAll(ctx, finlight.SearchParams{Query: "ticker:NVDA", Limit: 20}, &finlight.SearchAllOptions{MaxPages: 3})
	if err != nil {
		return err
	}
	count := 0
	for _, err := range res.All() {
		if err != nil {
			return err
		}
		count++
	}
	fmt.Fprintf(w, "%d articles on %d pages\n", count, res.PagesFetched())
	if res.Truncated() {
		fmt.Fprintf(w, "stopped at maxPages; continue with SearchParams{Cursor: %q} and the same filters\n", *res.NextCursor())
	}
	return nil
}
