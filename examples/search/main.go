// Command search fetches one page of articles about NVIDIA.
//
//	FINLIGHT_API_KEY=... go run ./examples/search
package main

import (
	"context"
	"errors"
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
		BaseURL: os.Getenv("FINLIGHT_BASE_URL"), // optional
		WSSURL:  os.Getenv("FINLIGHT_WSS_URL"),  // optional
	}
}

func run(ctx context.Context, w io.Writer, cfg finlight.Config) error {
	client, err := finlight.NewClient(cfg)
	if err != nil {
		return err
	}
	page, err := client.Articles.Search(ctx, finlight.SearchParams{
		Query:     "ticker:NVDA",
		Languages: []string{"en"},
		Include:   []finlight.IncludeBlock{finlight.IncludeBlockCompanies, finlight.IncludeBlockEvents},
		Limit:     10,
	})
	var apiErr *finlight.APIError
	if errors.As(err, &apiErr) {
		fmt.Fprintf(w, "API error: status %d, code %s, request %s\n", apiErr.Status, apiErr.Code, apiErr.RequestID)
		return err
	}
	if err != nil {
		return err
	}
	for _, a := range page.Articles {
		when := "unknown time"
		if t, ok := a.PublishedAtTime(); ok {
			when = t.UTC().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s  %-20s %s\n", when, a.Source.Domain, a.Title)
		if a.CompaniesPresent() {
			for _, c := range a.Companies.Value() {
				fmt.Fprintf(w, "    company %d %s\n", c.CompanyID, c.Name)
			}
		} else {
			fmt.Fprintln(w, "    companies not delivered (plan or include)")
		}
	}
	fmt.Fprintf(w, "hasMore: %v\n", page.Pagination.HasMore)
	return nil
}
