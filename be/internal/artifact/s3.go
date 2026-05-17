package artifact

import (
	"context"
	"errors"
	"io"
)

type s3Stub struct{}

func newS3Stub() *s3Stub { return &s3Stub{} }

func (s *s3Stub) Put(_ context.Context, _ string, _ io.Reader) error {
	return errors.New("s3 backend not yet implemented")
}

func (s *s3Stub) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, errors.New("s3 backend not yet implemented")
}

func (s *s3Stub) Delete(_ context.Context, _ string) error {
	return errors.New("s3 backend not yet implemented")
}
