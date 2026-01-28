package repository

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lucasew/fetchurl/internal/eviction"
	"github.com/lucasew/fetchurl/internal/hashutil"
	"golang.org/x/sync/singleflight"
)

// LocalRepository implements a Repository backed by the local filesystem.
//
// It uses a directory structure of {cacheDir}/{algo}/{hash} to store files.
// It integrates with the Eviction Manager to track usage and size.
type LocalRepository struct {
	CacheDir string
	eviction *eviction.Manager
	g        singleflight.Group
}

func NewLocalRepository(cacheDir string, eviction *eviction.Manager) *LocalRepository {
	return &LocalRepository{
		CacheDir: cacheDir,
		eviction: eviction,
	}
}

func (r *LocalRepository) getPath(algo, hash string) string {
	return filepath.Join(r.CacheDir, algo, hash)
}

func (r *LocalRepository) Exists(ctx context.Context, algo, hash string) (bool, error) {
	_, err := os.Stat(r.getPath(algo, hash))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (r *LocalRepository) Get(ctx context.Context, algo, hash string) (io.ReadCloser, int64, error) {
	path := r.getPath(algo, hash)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	if r.eviction != nil {
		r.eviction.Touch(filepath.Join(algo, hash))
	}
	return f, info.Size(), nil
}

// Put stores a file in the local cache if it doesn't already exist.
//
// It uses singleflight to ensure that multiple concurrent requests for the same hash
// only result in a single fetch/store operation.
//
// The process ensures data integrity:
// 1. Fetches content to a temporary file.
// 2. Computes the hash while writing.
// 3. Verifies the computed hash matches the requested hash.
// 4. Atomically moves (renames) the temporary file to the final location.
func (r *LocalRepository) Put(ctx context.Context, algo, hash string, fetcher Fetcher) error {
	key := filepath.Join(algo, hash)
	_, err, _ := r.g.Do(key, func() (interface{}, error) {
		// Double check existence
		if exists, _ := r.Exists(ctx, algo, hash); exists {
			return nil, nil
		}

		// Fetch
		reader, _, err := fetcher()
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()

		// Prepare destination
		finalPath := r.getPath(algo, hash)
		if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create algo dir: %w", err)
		}

		tmpFile, err := os.CreateTemp(r.CacheDir, "put-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer func() { _ = os.Remove(tmpFile.Name()) }()
		defer func() { _ = tmpFile.Close() }()

		hasher, err := hashutil.GetHasher(algo)
		if err != nil {
			return nil, err
		}

		mw := io.MultiWriter(tmpFile, hasher)
		written, err := io.Copy(mw, reader)
		if err != nil {
			return nil, fmt.Errorf("failed to write to temp file: %w", err)
		}

		// Verify hash
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != hash {
			return nil, fmt.Errorf("hash mismatch: expected %s, got %s", hash, actualHash)
		}

		if err := tmpFile.Close(); err != nil {
			return nil, fmt.Errorf("failed to close temp file: %w", err)
		}

		if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
			return nil, fmt.Errorf("failed to rename to final path: %w", err)
		}

		if r.eviction != nil {
			r.eviction.Add(key, written)
		}

		slog.Info("Stored file", "algo", algo, "hash", hash, "size", written)
		return nil, nil
	})
	return err
}

// GetOrFetch attempts to retrieve the file from the cache.
// If it's missing, it uses the provided fetcher to download and store it,
// then returns the file reader.
func (r *LocalRepository) GetOrFetch(ctx context.Context, algo, hash string, fetcher Fetcher) (io.ReadCloser, int64, error) {
	// 1. Try Local Cache (HIT)
	reader, size, err := r.Get(ctx, algo, hash)
	if err == nil {
		return reader, size, nil
	}

	// 2. Cache Miss -> Fetch & Store
	err = r.Put(ctx, algo, hash, fetcher)
	if err != nil {
		return nil, 0, err
	}

	// 3. Serve after successful Store
	return r.Get(ctx, algo, hash)
}
