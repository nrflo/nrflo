package artifact

import (
	"context"
	"fmt"
	"io"
)

// StorageMode identifies the backing store for artifacts.
type StorageMode string

const (
	ModeInternal   StorageMode = "internal"
	ModeS3         StorageMode = "s3"
	ModeR2         StorageMode = "cloudflare_r2"
)

// Storage is the I/O abstraction for artifact backends.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Config holds backend configuration for a Storage instance.
type Config struct {
	Mode StorageMode
}

// NewFromProjectConfig returns a Storage backend for the given project and config.
func NewFromProjectConfig(_ context.Context, projectID string, cfg Config) (Storage, error) {
	switch cfg.Mode {
	case ModeInternal:
		return newInternalFS(projectID), nil
	case ModeS3, ModeR2:
		return nil, fmt.Errorf("storage mode %q not implemented in T1", cfg.Mode)
	default:
		return nil, fmt.Errorf("unknown storage mode %q", cfg.Mode)
	}
}
