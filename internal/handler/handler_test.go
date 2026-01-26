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
	h := NewCASHandler(cacheDir)

	// Setup mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer upstream.Close()

	// Calculate hashes
	hash1 := sha256Sum([]byte("content1"))
	hash2 := sha256Sum([]byte("content2"))

	t.Run("Download Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/file1", hash1, upstream.URL), nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "content1" {
			t.Errorf("expected body content1, got %s", w.Body.String())
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
	})

	t.Run("Hash Mismatch New File", func(t *testing.T) {
		// Requesting hash2 but pointing to content1 (hash1)
		// hash2 matches "content2", but we point to "/file1" which returns "content1"
		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/file1", hash2, upstream.URL), nil)
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

		req := httptest.NewRequest("GET", fmt.Sprintf("/fetch/%s?url=%s/fail&url=%s/file2", hash2, upstream.URL, upstream.URL), nil)
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
}

func sha256Sum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
