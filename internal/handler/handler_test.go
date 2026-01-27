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

	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/lucasew/fetchurl/internal/repository"
)

func TestCASHandler(t *testing.T) {
	// Setup temporary cache dir
	cacheDir := t.TempDir()
	localRepo := repository.NewLocalRepository(cacheDir, nil)
	h := NewCASHandler(localRepo, fetcher.NewService(nil))

	// Setup mock upstream server (origin server for files)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file1":
			_, _ = w.Write([]byte("content1"))
		case "/file2":
			_, _ = w.Write([]byte("content2"))
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer origin.Close()

	// Calculate hashes
	hash1 := sha256Sum([]byte("content1"))
	hash2 := sha256Sum([]byte("content2"))

	t.Run("Download Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/sha256/%s?url=%s/file1", hash1, origin.URL), nil)
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

		// Verify file exists in cache
		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", hash1)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("Cache Hit", func(t *testing.T) {
		// Should be in cache from previous test
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/sha256/%s", hash1), nil)
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

	t.Run("Hash Mismatch New File", func(t *testing.T) {
		// Requesting hash2 but pointing to content1 (hash1)
		// hash2 matches "content2", but we point to "/file1" which returns "content1"
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/sha256/%s?url=%s/file1", hash2, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404 (failed to fetch/store), got %d. Body: %s", w.Code, w.Body.String())
		}

		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", hash2)); !os.IsNotExist(err) {
			t.Errorf("file should not exist in cache")
		}
	})

	t.Run("Failover", func(t *testing.T) {
		// First URL fails, second succeeds.
		// We use hash2 and file2. hash2 is NOT in cache because the previous test failed (as expected).

		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/sha256/%s?url=%s/fail&url=%s/file2", hash2, origin.URL, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "content2" {
			t.Errorf("expected body content2, got %s", w.Body.String())
		}

		// Verify file exists in cache
		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", hash2)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("HEAD Cache Miss", func(t *testing.T) {
		// New random hash, not in cache
		hash3 := sha256Sum([]byte("content3"))
		req := httptest.NewRequest("HEAD", fmt.Sprintf("/fetch/sha256/%s?url=%s/file1", hash3, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
		// Should NOT download
		if _, err := os.Stat(filepath.Join(cacheDir, "sha256", hash3)); !os.IsNotExist(err) {
			t.Errorf("file should not exist in cache")
		}
	})

	t.Run("HEAD Cache Hit", func(t *testing.T) {
		// hash1 is already in cache
		req := httptest.NewRequest("HEAD", fmt.Sprintf("/fetch/sha256/%s", hash1), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		// Content-Length should be set
		if w.Header().Get("Content-Length") == "" {
			t.Errorf("expected Content-Length header")
		}
	})
}

func TestUpstream(t *testing.T) {
	// Setup Server 2 (Upstream)
	upstreamCacheDir := t.TempDir()
	content := []byte("upstream-content")
	hash := sha256Sum(content)
	algo := "sha256"

	err := os.MkdirAll(filepath.Join(upstreamCacheDir, algo), 0755)
	if err != nil {
		t.Fatal(err)
	}
	upstreamFile := filepath.Join(upstreamCacheDir, algo, hash)
	_ = os.WriteFile(upstreamFile, content, 0644)

	upstreamLocal := repository.NewLocalRepository(upstreamCacheDir, nil)
	upstreamHandler := NewCASHandler(upstreamLocal, fetcher.NewService(nil))
	upstreamServer := httptest.NewServer(upstreamHandler)
	defer upstreamServer.Close()

	// Setup Server 1 (Local)
	localCacheDir := t.TempDir()
	localRepo := repository.NewLocalRepository(localCacheDir, nil)
	upstreamRepo := repository.NewUpstreamRepository(upstreamServer.URL)
	localHandler := NewCASHandler(localRepo, fetcher.NewService([]repository.Repository{upstreamRepo}))

	t.Run("Fetch from Upstream", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/sha256/%s", hash), nil)
		w := httptest.NewRecorder()

		localHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != string(content) {
			t.Errorf("expected body %s, got %s", content, w.Body.String())
		}

		// Verify file cached locally
		if _, err := os.Stat(filepath.Join(localCacheDir, "sha256", hash)); os.IsNotExist(err) {
			t.Errorf("file not found in local cache")
		}
	})
}

func sha256Sum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
