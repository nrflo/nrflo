package artifact

import (
	"bytes"
	"context"
	"testing"
)

func TestS3Stub_Put_Error(t *testing.T) {
	t.Parallel()
	stub := newS3Stub()
	err := stub.Put(context.Background(), "some/key.txt", bytes.NewReader([]byte("data")))
	if err == nil {
		t.Fatal("Put() expected error, got nil")
	}
	if err.Error() != "s3 backend not yet implemented" {
		t.Errorf("Put() error = %q, want %q", err.Error(), "s3 backend not yet implemented")
	}
}

func TestS3Stub_Get_Error(t *testing.T) {
	t.Parallel()
	stub := newS3Stub()
	rc, err := stub.Get(context.Background(), "some/key.txt")
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
	if rc != nil {
		t.Error("Get() expected nil reader on error")
	}
	if err.Error() != "s3 backend not yet implemented" {
		t.Errorf("Get() error = %q, want %q", err.Error(), "s3 backend not yet implemented")
	}
}

func TestS3Stub_Delete_Error(t *testing.T) {
	t.Parallel()
	stub := newS3Stub()
	err := stub.Delete(context.Background(), "some/key.txt")
	if err == nil {
		t.Fatal("Delete() expected error, got nil")
	}
	if err.Error() != "s3 backend not yet implemented" {
		t.Errorf("Delete() error = %q, want %q", err.Error(), "s3 backend not yet implemented")
	}
}

func TestS3Stub_ImplementsStorage(t *testing.T) {
	t.Parallel()
	var _ Storage = newS3Stub()
}
