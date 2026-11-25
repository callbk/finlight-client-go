// Command lookup fetches an article by link and by id, a company and the
// source list.
//
//	FINLIGHT_API_KEY=... go run ./examples/lookup <article link>
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
	link := "https://www.reuters.com/"
	if len(os.Args) > 1 {
		link = os.Args[1]
	}
	if err := run(context.Background(), os.Stdout, configFromEnv(), link); err != nil {
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

func run(ctx context.Context, w io.Writer, cfg finlight.Config, link string) error {
	client, err := finlight.NewClient(cfg)
	if err != nil {
		return err
	}

	// By link: reaches articles your plan covers (custom-source grant, plan date
	// window). A miss or a blocked article is a NOT_FOUND error, not nil.
	a, err := client.Articles.GetByLink(ctx, link, &finlight.GetOptions{Include: []finlight.IncludeBlock{finlight.IncludeBlockContent}})
	switch {
	case isNotFound(err):
		fmt.Fprintln(w, "no article with this link")
	case err != nil:
		return err
	default:
		fmt.Fprintf(w, "by link: %s (id %s)\n", a.Title, a.ID)
		if a.ContentPresent() {
			if c, ok := a.Content.Get(); ok {
				fmt.Fprintf(w, "content: %d bytes\n", len(c))
			} else {
				fmt.Fprintln(w, "content: none (null)")
			}
		} else {
			fmt.Fprintln(w, "content: not delivered (plan)")
		}
		// By id. Articles indexed before ids were introduced answer 404 here.
		if _, err := client.Articles.Get(ctx, a.ID, nil); err != nil && !isNotFound(err) {
			return err
		}
	}

	// Companies need entityAccess (else a 403 FORBIDDEN error).
	companies, err := client.Companies.Search(ctx, finlight.CompanySearchParams{Q: "NVDA", MIC: "XNAS"})
	if err != nil {
		return err
	}
	for _, c := range companies.Items {
		fmt.Fprintf(w, "company %d %s\n", c.CompanyID, c.Name)
		rec, err := client.Companies.Get(ctx, c.CompanyID, nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  etag %s, cache-control %s\n", rec.CacheInfo().ETag, rec.CacheInfo().CacheControl)
	}

	sources, err := client.Sources.List(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%d sources\n", len(sources.Items))
	return nil
}

func isNotFound(err error) bool {
	var apiErr *finlight.APIError
	return errors.As(err, &apiErr) && apiErr.APIKind == finlight.APIKindNotFound
}
