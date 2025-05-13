package db

import (
	"context"
	"time"
)

// Store define a interface para o armazenamento de dados do rate limiter.
type Store interface {
	Increment(ctx context.Context, key string, window time.Duration) (int64, error)
	IsBlocked(ctx context.Context, key string) (bool, error)
	Block(ctx context.Context, key string, duration time.Duration) error
	Reset(ctx context.Context, key string) error
	Close() error
}
