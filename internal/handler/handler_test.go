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
)

func TestCASHandler(t *testing.T) {
	// Setup temporary cache dir
	cacheDir := t.TempDir()
	h := NewCASHandler(cacheDir, nil)

	// Setup mock upstream server (origin server for files)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file1":
			w.Write([]byte("content1"))
		case "/file2":
			w.Write([]byte("content2"))
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
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/file1", hash1, origin.URL), nil)
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
		if w.Header().Get("Link") != fmt.Sprintf("</fetch/%s>; rel=\"canonical\"", hash1) {
			t.Errorf("expected Link canonical header, got %s", w.Header().Get("Link"))
		}

		// Verify file exists in cache
		if _, err := os.Stat(filepath.Join(cacheDir, hash1)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("Cache Hit", func(t *testing.T) {
		// Should be in cache from previous test
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s", hash1), nil)
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
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/file1", hash2, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500 (mismatch), got %d. Body: %s", w.Code, w.Body.String())
		}

		if _, err := os.Stat(filepath.Join(cacheDir, hash2)); !os.IsNotExist(err) {
			t.Errorf("file should not exist in cache")
		}
	})

	t.Run("Failover", func(t *testing.T) {
		// First URL fails, second succeeds.
		// We use hash2 and file2. hash2 is NOT in cache because the previous test failed (as expected).

		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/fail&url=%s/file2", hash2, origin.URL, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "content2" {
			t.Errorf("expected body content2, got %s", w.Body.String())
		}

		// Verify file exists in cache
		if _, err := os.Stat(filepath.Join(cacheDir, hash2)); os.IsNotExist(err) {
			t.Errorf("file not found in cache")
		}
	})

	t.Run("HEAD Cache Miss", func(t *testing.T) {
		// New random hash, not in cache
		hash3 := sha256Sum([]byte("content3"))
		req := httptest.NewRequest("HEAD", fmt.Sprintf("/fetch/%s?url=%s/file1", hash3, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
		// Should NOT download
		if _, err := os.Stat(filepath.Join(cacheDir, hash3)); !os.IsNotExist(err) {
			t.Errorf("file should not exist in cache")
		}
	})

	t.Run("HEAD Cache Hit", func(t *testing.T) {
		// hash1 is already in cache
		req := httptest.NewRequest("HEAD", fmt.Sprintf("/fetch/%s", hash1), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		// Content-Length should be set by ServeFile
		if w.Header().Get("Content-Length") == "" {
			t.Errorf("expected Content-Length header")
		}
	})

	t.Run("Error No Cache Headers", func(t *testing.T) {
		// Non-existent hash
		hash4 := sha256Sum([]byte("content4"))
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/fail", hash4, origin.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", w.Code)
		}

		// Verify no cache headers
		if w.Header().Get("Cache-Control") != "" {
			t.Errorf("expected no Cache-Control header, got %s", w.Header().Get("Cache-Control"))
		}
		if w.Header().Get("Link") != "" {
			t.Errorf("expected no Link header, got %s", w.Header().Get("Link"))
		}
	})
}

func TestUpstream(t *testing.T) {
	// Upstream logic:
	// 1. Client requests File A from Server 1 (Local).
	// 2. Server 1 doesn't have File A.
	// 3. Server 1 is configured with Server 2 (Upstream) as upstream.
	// 4. Server 1 asks Server 2 for File A.
	// 5. Server 2 has File A (or fetches it).
	// 6. Server 1 downloads File A from Server 2, caches it, and serves it.

	// Setup Server 2 (Upstream)
	upstreamCacheDir := t.TempDir()
	// Create a file in upstream cache
	content := []byte("upstream-content")
	hash := sha256Sum(content)
	upstreamFile := filepath.Join(upstreamCacheDir, hash)
	os.WriteFile(upstreamFile, content, 0644)

	upstreamHandler := NewCASHandler(upstreamCacheDir, nil)
	upstreamServer := httptest.NewServer(upstreamHandler)
	defer upstreamServer.Close()

	// Setup Server 1 (Local)
	localCacheDir := t.TempDir()
	// Configure upstream
	localHandler := NewCASHandler(localCacheDir, []string{upstreamServer.URL})

	t.Run("Fetch from Upstream", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s", hash), nil)
		w := httptest.NewRecorder()

		localHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != string(content) {
			t.Errorf("expected body %s, got %s", content, w.Body.String())
		}

		// Verify file cached locally
		if _, err := os.Stat(filepath.Join(localCacheDir, hash)); os.IsNotExist(err) {
			t.Errorf("file not found in local cache")
		}
	})

	t.Run("Upstream Miss Fallback", func(t *testing.T) {
		// Hash that upstream doesn't have
		content2 := []byte("content2")
		hash2 := sha256Sum(content2)

		// Create origin server that has the file
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/file2" {
				w.Write(content2)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer origin.Close()

		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/file2", hash2, origin.URL), nil)
		w := httptest.NewRecorder()

		localHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != string(content2) {
			t.Errorf("expected body %s, got %s", content2, w.Body.String())
		}

		// Should be cached locally now
		if _, err := os.Stat(filepath.Join(localCacheDir, hash2)); os.IsNotExist(err) {
			t.Errorf("file not found in local cache")
		}
	})
}

func sha256Sum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
