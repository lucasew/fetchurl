package handler

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/lucasew/fetchurl/internal/hashutil"
	"github.com/lucasew/fetchurl/internal/repository"
	"golang.org/x/sync/singleflight"
)

type CASHandler struct {
	Local  *repository.LocalRepository
	Client *http.Client
	g      singleflight.Group
}

func NewCASHandler(local *repository.LocalRepository, client *http.Client) *CASHandler {
	if client == nil {
		client = http.DefaultClient
	}
	return &CASHandler{
		Local:  local,
		Client: client,
	}
}

func (h *CASHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Expected path: /{algo}/{hash} (stripped prefix)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid path format. Expected /{algo}/{hash}", http.StatusBadRequest)
		return
	}
	algo := parts[0]
	hash := parts[1]

	if !hashutil.IsSupported(algo) {
		http.Error(w, fmt.Sprintf("Unsupported hash algorithm: %s", algo), http.StatusBadRequest)
		return
	}

	// 1. Try Local Cache
	exists, err := h.Local.Exists(r.Context(), algo, hash)
	if err != nil {
		slog.Error("Failed to check cache existence", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if exists {
		h.serveFromCache(w, r, algo, hash)
		return
	}

	// 2. Cache Miss -> Fetch & Stream
	sources := h.parseSourceUrls(r.Header)
	if len(sources) == 0 {
		http.Error(w, "Not found and no X-Source-Urls provided", http.StatusNotFound)
		return
	}

	sfKey := algo + ":" + hash

	// Capture if headers were written inside the leader execution
	headersWritten := false

	_, err, shared := h.g.Do(sfKey, func() (interface{}, error) {
		err := h.fetchAndStream(r.Context(), w, algo, hash, sources, &headersWritten)
		return nil, err
	})

	if err != nil {
		// If error occurred and we haven't written headers yet, send error response
		if !headersWritten {
			slog.Error("Fetch failed", "error", err)
			http.Error(w, fmt.Sprintf("Failed to fetch: %v", err), http.StatusBadGateway)
		} else {
			// Headers already written, connection might be aborted or partial.
			slog.Error("Fetch failed after headers written", "error", err)
		}
		return
	}

	// If shared, it means we waited for the leader.
	if shared {
		// Leader finished successfully. Serve from cache.
		h.serveFromCache(w, r, algo, hash)
	}
}

func (h *CASHandler) serveFromCache(w http.ResponseWriter, r *http.Request, algo, hash string) {
	reader, size, err := h.Local.Get(r.Context(), algo, hash)
	if err != nil {
		slog.Error("Failed to get from cache", "hash", hash, "error", err)
		http.Error(w, "Failed to retrieve from cache", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	h.setCacheHeaders(w, algo, hash)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	io.Copy(w, reader)
}

func (h *CASHandler) fetchAndStream(ctx context.Context, w http.ResponseWriter, algo, hash string, sources []string, headersWritten *bool) error {
	for _, source := range sources {
		slog.Info("Fetching from source", "url", source, "hash", hash)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			slog.Warn("Invalid source URL", "url", source, "error", err)
			continue
		}

		// Forward X-Source-Urls to allow daisy chaining
		for _, s := range sources {
			req.Header.Add("X-Source-Urls", s)
		}

		resp, err := h.Client.Do(req)
		if err != nil {
			slog.Warn("Failed to fetch from source", "url", source, "error", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			slog.Warn("Source returned non-200", "url", source, "status", resp.StatusCode)
			continue
		}

		// Found it! Start streaming.

		// 1. Prepare Storage
		tmpFile, commit, err := h.Local.BeginWrite(algo, hash)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}

		defer func() {
			tmpFile.Close()
			if f, ok := tmpFile.(*os.File); ok {
				os.Remove(f.Name())
			}
		}()

		// 2. Set Headers
		h.setCacheHeaders(w, algo, hash)
		if resp.ContentLength > 0 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
		}
		w.WriteHeader(http.StatusOK)
		*headersWritten = true

		// 3. Stream
		hasher, err := hashutil.GetHasher(algo)
		if err != nil {
             return err
        }

		mw := io.MultiWriter(w, tmpFile, hasher)

		written, err := io.Copy(mw, resp.Body)
		if err != nil {
			return fmt.Errorf("streaming failed: %w", err)
		}

		// 4. Verify Hash
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != hash {
			slog.Error("Hash mismatch", "expected", hash, "got", actualHash)
			panic(http.ErrAbortHandler)
		}

        if resp.ContentLength > 0 && written != resp.ContentLength {
             slog.Warn("Size mismatch", "expected", resp.ContentLength, "got", written)
             panic(http.ErrAbortHandler)
        }

		// 5. Commit
		if err := commit(); err != nil {
			slog.Error("Failed to commit file", "error", err)
			return nil
		}

		return nil // Success
	}

	return fmt.Errorf("all sources failed")
}

func (h *CASHandler) parseSourceUrls(headers http.Header) []string {
	var urls []string
	for _, v := range headers.Values("X-Source-Urls") {
		for _, u := range strings.Split(v, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				urls = append(urls, u)
			}
		}
	}
	return urls
}

func (h *CASHandler) setCacheHeaders(w http.ResponseWriter, algo, hash string) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Link", fmt.Sprintf("</fetch/%s/%s>; rel=\"canonical\"", algo, hash))
}
