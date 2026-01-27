package fetcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/lucasew/fetchurl/internal/repository"
)

type Fetcher interface {
	Fetch(ctx context.Context, algo, hash string, queryUrls []string) (io.ReadCloser, int64, error)
}

type Service struct {
	Upstreams []repository.Repository
}

func NewService(upstreams []repository.Repository) *Service {
	return &Service{
		Upstreams: upstreams,
	}
}

func (s *Service) Fetch(ctx context.Context, algo, hash string, queryUrls []string) (io.ReadCloser, int64, error) {
	// Try upstreams first
	for _, upstream := range s.Upstreams {
		reader, size, err := upstream.Get(ctx, algo, hash)
		if err == nil {
			return reader, size, nil
		}
	}

	// Try query URLs
	for _, u := range queryUrls {
		slog.Info("Downloading from query URL", "url", u, "algo", algo, "hash", hash)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
if err != nil {
	slog.Warn("Failed to create request for query URL", "url", u, "error", err)
	continue
}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			continue
		}
		return resp.Body, resp.ContentLength, nil
	}

	return nil, 0, fmt.Errorf("hash not found in upstreams or query URLs")
}
