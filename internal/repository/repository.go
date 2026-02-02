package repository

import (
	"context"
	"io"
)

type Repository interface {
	Exists(ctx context.Context, algo, hash string) (bool, error)
	Get(ctx context.Context, algo, hash string) (io.ReadCloser, int64, error)
}
