package finlight

import (
	"context"
	"net/http"
	"net/url"
)

// ArticleService fetches financial news articles.
type ArticleService struct {
	api *apiClient
}

// FetchArticles searches articles matching params and returns one result page.
func (s *ArticleService) FetchArticles(ctx context.Context, params GetArticlesParams) (*ArticleResponse, error) {
	var resp ArticleResponse
	if err := s.api.do(ctx, http.MethodPost, "/v2/articles", nil, params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// FetchArticleByLink fetches a single article by its URL.
func (s *ArticleService) FetchArticleByLink(ctx context.Context, params GetArticleByLinkParams) (*Article, error) {
	query := url.Values{"link": {params.Link}}
	if params.IncludeContent {
		query.Set("includeContent", "true")
	}
	if params.IncludeEntities {
		query.Set("includeEntities", "true")
	}
	var envelope struct {
		Article Article `json:"article"`
	}
	if err := s.api.do(ctx, http.MethodGet, "/v2/articles/by-link", query, nil, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Article, nil
}
