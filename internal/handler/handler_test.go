package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/fetchurl/internal/repository"
)

func TestCASHandler(t *testing.T) {
	// Setup temporary cache dir
	cacheDir := t.TempDir()
	localRepo := repository.NewLocalRepository(cacheDir, nil)
	// We use the default client for the handler
	h := NewCASHandler(localRepo, nil, nil)

	// Setup mock upstream server (origin server for files)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file1":
			_, _ = w.Write([]byte("content1"))
		case "/file2":
			_, _ = w.Write([]byte("content2"))
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
		case "/big":
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("0123456789"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer origin.Close()

	// Calculate hashes
	hash1 := sha256Sum([]byte("content1"))
	hash2 := sha256Sum([]byte("content2"))

	t.Run("Download Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/sha256/%s", hash1), nil)
		req.Header.Set("X-Source-Urls", origin.URL+"/file1")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "content1" {
			t.Errorf("expected body content1, got %s", w.Body.String())
		}

		// Verify headers
		if w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("expected Cache-Control header, got %s", w.Header().Get("Cache-Control"))
		}
		if w.Header().Get("Link") != fmt.Sprintf("</fetch/sha256/%s>; rel=\"canonical\"", hash1) {
			t.Errorf("expected Link canonical header, got %s", w.Header().Get("Link"))
		}

		// Verify file exists in cache (sharded)
		shard := hash1[:2]
		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", shard, hash1)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("Cache Hit", func(t *testing.T) {
		// Should be in cache from previous test
		req := httptest.NewRequest("GET", fmt.Sprintf("/sha256/%s", hash1), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if w.Body.String() != "content1" {
			t.Errorf("expected body content1, got %s", w.Body.String())
		}
		// Verify headers on cache hit
		if w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("expected Cache-Control header, got %s", w.Header().Get("Cache-Control"))
		}
	})

	t.Run("Hash Mismatch", func(t *testing.T) {
		// Requesting hash2 but pointing to content1 (hash1)

		defer func() {
			if r := recover(); r != nil {
				// Expected panic
				// We don't verify specific panic because singleflight wraps it.
			} else {
				t.Errorf("expected panic for hash mismatch")
			}
		}()

		req := httptest.NewRequest("GET", fmt.Sprintf("/sha256/%s", hash2), nil)
		req.Header.Set("X-Source-Urls", origin.URL+"/file1")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)
	})

	t.Run("Failover", func(t *testing.T) {
		// First URL fails, second succeeds.
		// hash2

		req := httptest.NewRequest("GET", fmt.Sprintf("/sha256/%s", hash2), nil)
		// Header with multiple sources
		req.Header.Add("X-Source-Urls", origin.URL+"/fail")
		req.Header.Add("X-Source-Urls", origin.URL+"/file2")
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "content2" {
			t.Errorf("expected body content2, got %s", w.Body.String())
		}

		// Verify file exists in cache
		shard := hash2[:2]
		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", shard, hash2)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("Missing X-Source-Urls", func(t *testing.T) {
		hash3 := sha256Sum([]byte("content3"))
		req := httptest.NewRequest("GET", fmt.Sprintf("/sha256/%s", hash3), nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}

func sha256Sum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
