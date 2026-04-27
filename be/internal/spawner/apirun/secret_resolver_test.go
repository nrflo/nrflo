package apirun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDereferenceSecretRef_EnvSuccess(t *testing.T) {
	t.Setenv("APIRUN_TEST_SECRET", "abc123")
	v, err := DereferenceSecretRef(context.Background(), "env:APIRUN_TEST_SECRET")
	if err != nil {
		t.Fatalf("DereferenceSecretRef: %v", err)
	}
	if v != "abc123" {
		t.Errorf("got %q, want %q", v, "abc123")
	}
}

func TestDereferenceSecretRef_EnvMissing(t *testing.T) {
	os.Unsetenv("APIRUN_TEST_MISSING")
	_, err := DereferenceSecretRef(context.Background(), "env:APIRUN_TEST_MISSING")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("err = %q, want substring 'is empty'", err.Error())
	}
}

func TestDereferenceSecretRef_EnvEmptyName(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "env:")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "env name empty") {
		t.Errorf("err = %q, want 'env name empty'", err.Error())
	}
}

func TestDereferenceSecretRef_FileSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("filevalue\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	v, err := DereferenceSecretRef(context.Background(), "file:"+path)
	if err != nil {
		t.Fatalf("DereferenceSecretRef: %v", err)
	}
	if v != "filevalue" {
		t.Errorf("got %q, want %q (whitespace trimmed)", v, "filevalue")
	}
}

func TestDereferenceSecretRef_FileMissing(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "file:/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read secret_ref file") {
		t.Errorf("err = %q, want substring 'read secret_ref file'", err.Error())
	}
}

func TestDereferenceSecretRef_FileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := DereferenceSecretRef(context.Background(), "file:"+path)
	if err == nil {
		t.Fatalf("expected empty-file error, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("err = %q, want substring 'is empty'", err.Error())
	}
}

func TestDereferenceSecretRef_FileEmptyPath(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "file:")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "file path empty") {
		t.Errorf("err = %q, want substring 'file path empty'", err.Error())
	}
}

func TestDereferenceSecretRef_LiteralSuccess(t *testing.T) {
	v, err := DereferenceSecretRef(context.Background(), "literal:hello")
	if err != nil {
		t.Fatalf("DereferenceSecretRef: %v", err)
	}
	if v != "hello" {
		t.Errorf("got %q, want %q", v, "hello")
	}
}

func TestDereferenceSecretRef_LiteralEmpty(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "literal:")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "literal value empty") {
		t.Errorf("err = %q, want substring 'literal value empty'", err.Error())
	}
}

func TestDereferenceSecretRef_UnsupportedScheme(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "vault:secret/data")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported secret_ref scheme") {
		t.Errorf("err = %q, want 'unsupported secret_ref scheme'", err.Error())
	}
}

func TestDereferenceSecretRef_NoColon(t *testing.T) {
	_, err := DereferenceSecretRef(context.Background(), "no_scheme_here")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
