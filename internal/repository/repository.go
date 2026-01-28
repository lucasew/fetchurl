package repository

import (
	"context"
	"io"
)

type Fetcher func() (io.ReadCloser, int64, error)

type Repository interface {
	Exists(ctx context.Context, algo, hash string) (bool, error)
	Get(ctx context.Context, algo, hash string) (io.ReadCloser, int64, error)
}

type WritableRepository interface {
	Repository
	Put(ctx context.Context, algo, hash string, fetcher Fetcher) error
	GetOrFetch(ctx context.Context, algo, hash string, fetcher Fetcher) (io.ReadCloser, int64, error)
}
