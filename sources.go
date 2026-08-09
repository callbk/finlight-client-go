package finlight

import (
	"context"
	"net/http"
)

// SourceService lists the news sources available through the API.
type SourceService struct {
	api *apiClient
}

// GetSources returns all sources with their availability flags.
func (s *SourceService) GetSources(ctx context.Context) ([]Source, error) {
	var sources []Source
	if err := s.api.do(ctx, http.MethodGet, "/v2/sources", nil, nil, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}
