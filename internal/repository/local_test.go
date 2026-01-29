package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestLocalRepository_GetOrFetch(t *testing.T) {
	cacheDir := t.TempDir()
	repo := NewLocalRepository(cacheDir, nil)
	ctx := context.Background()
	algo := "sha256"

	content := "test content"
	h := sha256.New()
	h.Write([]byte(content))
	hash := hex.EncodeToString(h.Sum(nil))

	t.Run("Cache Miss Success", func(t *testing.T) {
		fetchCalled := false
		fetcher := func() (io.ReadCloser, int64, error) {
			fetchCalled = true
			return io.NopCloser(strings.NewReader(content)), int64(len(content)), nil
		}

		rc, size, err := repo.GetOrFetch(ctx, algo, hash, fetcher)
		if err != nil {
			t.Fatalf("GetOrFetch failed: %v", err)
		}
		defer func() { _ = rc.Close() }()

		if !fetchCalled {
			t.Error("Fetcher was not called on cache miss")
		}
		if size != int64(len(content)) {
			t.Errorf("Expected size %d, got %d", len(content), size)
		}

		bytes, _ := io.ReadAll(rc)
		if string(bytes) != content {
			t.Errorf("Expected content %q, got %q", content, string(bytes))
		}
	})

	t.Run("Cache Hit", func(t *testing.T) {
		// File should already be there from previous test
		fetchCalled := false
		fetcher := func() (io.ReadCloser, int64, error) {
			fetchCalled = true
			return io.NopCloser(strings.NewReader("")), 0, nil
		}

		rc, size, err := repo.GetOrFetch(ctx, algo, hash, fetcher)
		if err != nil {
			t.Fatalf("GetOrFetch failed: %v", err)
		}
		defer func() { _ = rc.Close() }()

		if fetchCalled {
			t.Error("Fetcher WAS called on cache hit")
		}
		if size != int64(len(content)) {
			t.Errorf("Expected size %d, got %d", len(content), size)
		}
	})

	t.Run("Fetch Error", func(t *testing.T) {
		newHash := "0000000000000000000000000000000000000000000000000000000000000000"
		fetcher := func() (io.ReadCloser, int64, error) {
			return nil, 0, io.ErrUnexpectedEOF
		}

		_, _, err := repo.GetOrFetch(ctx, algo, newHash, fetcher)
		if err != io.ErrUnexpectedEOF {
			t.Errorf("Expected ErrUnexpectedEOF, got %v", err)
		}
	})

    t.Run("Hash Mismatch", func(t *testing.T) {
        // Requesting a hash, but fetcher returns content that doesn't match
        reqHash := "1111111111111111111111111111111111111111111111111111111111111111"
		fetcher := func() (io.ReadCloser, int64, error) {
			return io.NopCloser(strings.NewReader(content)), int64(len(content)), nil
		}

        _, _, err := repo.GetOrFetch(ctx, algo, reqHash, fetcher)
        if err == nil {
            t.Error("Expected error on hash mismatch, got nil")
        }
        if !strings.Contains(err.Error(), "hash mismatch") {
             t.Errorf("Expected 'hash mismatch' error, got %v", err)
        }
    })
}
