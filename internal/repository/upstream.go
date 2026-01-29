package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UpstreamRepository accesses a remote CAS server (another fetchurl instance).
//
// It allows for federation and cache tiering by delegating requests to other servers.
type UpstreamRepository struct {
	BaseURL string
	Client  *http.Client
}

func NewUpstreamRepository(baseURL string, client *http.Client) *UpstreamRepository {
	if client == nil {
		client = http.DefaultClient
	}
	return &UpstreamRepository{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  client,
	}
}

// Exists checks if the file exists on the upstream server using a HEAD request.
func (r *UpstreamRepository) Exists(ctx context.Context, algo, hash string) (bool, error) {
	url := fmt.Sprintf("%s/fetch/%s/%s", r.BaseURL, algo, hash)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK, nil
}

func (r *UpstreamRepository) Get(ctx context.Context, algo, hash string) (io.ReadCloser, int64, error) {
	url := fmt.Sprintf("%s/fetch/%s/%s", r.BaseURL, algo, hash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	return resp.Body, resp.ContentLength, nil
}
