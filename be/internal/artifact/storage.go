package artifact

import (
	"context"
	"fmt"
	"io"
)

// StorageMode identifies the backing store for artifacts.
type StorageMode string

const (
	ModeInternal StorageMode = "internal"
	ModeS3       StorageMode = "s3"
	ModeR2       StorageMode = "cloudflare_r2"
)

// Storage is the I/O abstraction for artifact backends.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Config holds backend configuration for a Storage instance.
type Config struct {
	Mode         StorageMode `json:"mode"`
	AccountID    string      `json:"account_id,omitempty"`
	Bucket       string      `json:"bucket,omitempty"`
	Prefix       string      `json:"prefix,omitempty"`
	AccessKeyRef string      `json:"access_key_ref,omitempty"`
	SecretKeyRef string      `json:"secret_key_ref,omitempty"`
}

// NewFromProjectConfig returns a Storage backend for the given project and config.
func NewFromProjectConfig(ctx context.Context, projectID string, cfg Config) (Storage, error) {
	switch cfg.Mode {
	case ModeInternal:
		return newInternalFS(projectID), nil
	case ModeS3:
		return newS3Stub(), nil
	case ModeR2:
		return newR2(ctx, projectID, cfg)
	default:
		return nil, fmt.Errorf("unknown storage mode %q", cfg.Mode)
	}
}
