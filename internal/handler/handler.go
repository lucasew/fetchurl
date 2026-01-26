package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/singleflight"
)

type CASHandler struct {
	CacheDir string
	g        singleflight.Group
}

func NewCASHandler(cacheDir string) *CASHandler {
	return &CASHandler{
		CacheDir: cacheDir,
	}
}

func (h *CASHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Expected path: /fetch/{hash}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[1] != "fetch" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	hash := parts[2]

	// Validate hash format (sha256 hex string)
	if len(hash) != 64 {
		http.Error(w, "Invalid hash format", http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(hash); err != nil {
		http.Error(w, "Invalid hash characters", http.StatusBadRequest)
		return
	}

	cachePath := filepath.Join(h.CacheDir, hash)

	// Check cache
	if _, err := os.Stat(cachePath); err == nil {
		http.ServeFile(w, r, cachePath)
		return
	}

	// If method is HEAD and cache miss, return 404
	if r.Method == http.MethodHead {
		http.Error(w, "File not found in cache", http.StatusNotFound)
		return
	}

	// Not in cache, attempt download
	urls := r.URL.Query()["url"]
	if len(urls) == 0 {
		http.Error(w, "File not found in cache and no URLs provided", http.StatusNotFound)
		return
	}

	_, err, _ := h.g.Do(hash, func() (interface{}, error) {
		// Double check cache inside singleflight
		if _, err := os.Stat(cachePath); err == nil {
			return nil, nil
		}

		return nil, h.download(hash, urls)
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch: %v", err), http.StatusInternalServerError)
		return
	}

	http.ServeFile(w, r, cachePath)
}

func (h *CASHandler) download(expectedHash string, urls []string) error {
	var lastErr error

	for _, u := range urls {
		err := h.downloadOne(expectedHash, u)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("all sources failed, last error: %w", lastErr)
	}
	return errors.New("no sources provided")
}

func (h *CASHandler) downloadOne(expectedHash string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d from %s", resp.StatusCode, url)
	}

	// Create temp file
	if err := os.MkdirAll(h.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(h.CacheDir, "download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up on error/return (if not renamed)
	defer tmpFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		return fmt.Errorf("failed to copy body: %w", err)
	}

	// Verify hash
	downloadedHash := hex.EncodeToString(hasher.Sum(nil))
	if downloadedHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, downloadedHash)
	}

	// Close file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	finalPath := filepath.Join(h.CacheDir, expectedHash)
	if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	// Prevent defer os.Remove from deleting the file we just renamed?
	// os.Rename moves the file, so the old path (tmpFile.Name()) no longer exists.
	// os.Remove on a non-existent file returns an error, but defer ignores it usually.
	// But to be clean, we rely on the fact that if Rename succeeds, the temp file is gone.
	// os.Remove will return an error (PathError) which is fine to ignore in defer.

	return nil
}
