package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lucasew/fetchurl/internal/hashutil"
	"github.com/lucasew/fetchurl/internal/repository"
)

type CASHandler struct {
	Local     repository.WritableRepository
	Upstreams []repository.Repository
}

func NewCASHandler(local repository.WritableRepository, upstreams []repository.Repository) *CASHandler {
	return &CASHandler{
		Local:     local,
		Upstreams: upstreams,
	}
}

func (h *CASHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Expected path: /fetch/{algo}/{hash}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "fetch" {
		http.Error(w, "Invalid path format. Expected /fetch/{algo}/{hash}", http.StatusBadRequest)
		return
	}
	algo := parts[1]
	hash := parts[2]

	if !hashutil.IsSupported(algo) {
		http.Error(w, fmt.Sprintf("Unsupported hash algorithm: %s", algo), http.StatusBadRequest)
		return
	}

	// Try local first
	reader, size, err := h.Local.Get(r.Context(), algo, hash)
	if err == nil {
		defer reader.Close()
		slog.Debug("Cache hit", "algo", algo, "hash", hash)
		h.setCacheHeaders(w, algo, hash)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		io.Copy(w, reader)
		return
	}

	slog.Info("Cache miss", "algo", algo, "hash", hash)

	if r.Method == http.MethodHead {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Not in local, try to Put
	queryUrls := r.URL.Query()["url"]

	fetcher := func() (io.ReadCloser, int64, error) {
		// Try upstreams first
		for _, upstream := range h.Upstreams {
			reader, size, err := upstream.Get(r.Context(), algo, hash)
			if err == nil {
				return reader, size, nil
			}
		}

		// Try query URLs
		for _, u := range queryUrls {
			slog.Info("Downloading from query URL", "url", u, "algo", algo, "hash", hash)
			resp, err := http.Get(u)
			if err != nil {
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}
			return resp.Body, resp.ContentLength, nil
		}

		return nil, 0, fmt.Errorf("hash not found in upstreams or query URLs")
	}

	err = h.Local.Put(r.Context(), algo, hash, fetcher)
	if err != nil {
		slog.Error("Failed to fetch/store", "algo", algo, "hash", hash, "error", err)
		http.Error(w, fmt.Sprintf("Failed to fetch: %v", err), http.StatusNotFound)
		return
	}

	// Serve after Put
	reader, size, err = h.Local.Get(r.Context(), algo, hash)
	if err != nil {
		http.Error(w, "Failed to retrieve after store", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	h.setCacheHeaders(w, algo, hash)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	io.Copy(w, reader)
}

func (h *CASHandler) setCacheHeaders(w http.ResponseWriter, algo, hash string) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Link", fmt.Sprintf("</fetch/%s/%s>; rel=\"canonical\"", algo, hash))
}
