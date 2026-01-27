package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/lucasew/fetchurl/internal/hashutil"
	"github.com/lucasew/fetchurl/internal/repository"
)

// CASHandler (Content Addressable Storage Handler) serves files based on their hash.
//
// It implements a tiered lookup strategy:
// 1. Local Cache: Checks if the file exists locally.
// 2. Upstream Cache: Checks configured upstream servers.
// 3. External URL: If ?url=... is provided, fetches, verifies hash, and caches.
type CASHandler struct {
	Local   repository.WritableRepository
	Fetcher fetcher.Fetcher
}

func NewCASHandler(local repository.WritableRepository, fetcher fetcher.Fetcher) *CASHandler {
	return &CASHandler{
		Local:   local,
		Fetcher: fetcher,
	}
}

// ServeHTTP handles the /fetch/{algo}/{hash} requests.
//
// Flow:
// 1. Validates path and hash support.
// 2. Serves from local cache if available (HIT).
// 3. If MISS, attempts to fetch from upstreams or provided "url" query parameters.
// 4. On successful fetch, verifies the hash, stores in local cache, and serves the content.
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
		defer func() { _ = reader.Close() }()
		slog.Debug("Cache hit", "algo", algo, "hash", hash)
		h.setCacheHeaders(w, algo, hash)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		_, _ = io.Copy(w, reader)
		return
	}

	slog.Info("Cache miss", "algo", algo, "hash", hash)

	if r.Method == http.MethodHead {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Not in local, try to Put
	queryUrls := r.URL.Query()["url"]

	fetchFn := func() (io.ReadCloser, int64, error) {
		return h.Fetcher.Fetch(r.Context(), algo, hash, queryUrls)
	}

	err = h.Local.Put(r.Context(), algo, hash, fetchFn)
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
	defer func() { _ = reader.Close() }()

	h.setCacheHeaders(w, algo, hash)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	_, _ = io.Copy(w, reader)
}

// setCacheHeaders sets the HTTP headers for immutable caching.
//
// Since content is addressed by its hash, it can be cached indefinitely (immutable).
// It also sets the canonical Link header to the CAS URL.
func (h *CASHandler) setCacheHeaders(w http.ResponseWriter, algo, hash string) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Link", fmt.Sprintf("</fetch/%s/%s>; rel=\"canonical\"", algo, hash))
}
