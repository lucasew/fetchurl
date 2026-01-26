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

type LocalRepository struct {
	CacheDir string
	eviction *eviction.Manager
	g        singleflight.Group
}

// Ensure LocalRepository implements eviction.Store
var _ eviction.Store = (*LocalRepository)(nil)

func NewLocalRepository(cacheDir string, eviction *eviction.Manager) *LocalRepository {
	return &LocalRepository{
		CacheDir: cacheDir,
		eviction: eviction,
	}
}

func (r *LocalRepository) Walk(fn func(key string, size int64) error) error {
	return filepath.WalkDir(r.CacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == r.CacheDir {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			slog.Warn("Failed to get file info", "file", path, "error", err)
			return nil
		}

		rel, err := filepath.Rel(r.CacheDir, path)
		if err != nil {
			slog.Warn("Failed to get relative path", "path", path, "error", err)
			return nil
		}

		return fn(rel, info.Size())
	})
}

func (r *LocalRepository) Delete(key string) error {
	path := filepath.Join(r.CacheDir, key)
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
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
		f.Close()
		return nil, 0, err
	}
	if r.eviction != nil {
		r.eviction.Touch(filepath.Join(algo, hash))
	}
	return f, info.Size(), nil
}

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
		defer reader.Close()

		// Prepare destination
		finalPath := r.getPath(algo, hash)
		if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create algo dir: %w", err)
		}

		tmpFile, err := os.CreateTemp(r.CacheDir, "put-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

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
