package anthropic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/model"
)

func TestResolveAPIKey_SecretRef_EnvErrors(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	tests := []struct {
		name      string
		secretRef string
		wantSub   string
	}{
		{"empty name", "env:", "empty"},
		{"missing var", "env:NRFLO_TEST_DOES_NOT_EXIST_XYZ", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("NRFLO_TEST_DOES_NOT_EXIST_XYZ")
			repo := &fakeRepo{rows: map[string]*model.APICredential{
				"anthropic|": {SecretRef: tt.secretRef},
			}}
			_, err := ResolveAPIKey(context.Background(), repo, nil, "")
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestResolveAPIKey_SecretRef_FileMissing(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	missing := filepath.Join(t.TempDir(), "does-not-exist.key")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "file:" + missing},
	}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read api_credentials file") {
		t.Errorf("err = %q, want it to mention file read", err.Error())
	}
}

func TestResolveAPIKey_SecretRef_FileEmpty(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(empty, []byte("   \n\t  "), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "file:" + empty},
	}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("err = %q, want it to mention 'is empty'", err.Error())
	}
}

func TestResolveAPIKey_SecretRef_FileOK(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("\nsk-from-file\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "file:" + path},
	}}
	got, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got.Value != "sk-from-file" {
		t.Errorf("got %q, want %q", got.Value, "sk-from-file")
	}
}

func TestResolveAPIKey_SecretRef_FileEmptyPathErrors(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "file:"},
	}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error for empty file path")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %q, want substring 'empty'", err.Error())
	}
}

func TestResolveAPIKey_SecretRef_LiteralWarnsOnce(t *testing.T) {
	resetLiteralWarned(t)
	buf := captureLogger(t)
	t.Setenv("ANTHROPIC_API_KEY", "")

	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "literal:sk-abc"},
	}}

	got, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey #1: %v", err)
	}
	if got.Value != "sk-abc" {
		t.Errorf("got %q, want %q", got.Value, "sk-abc")
	}
	out1 := buf.String()
	if !strings.Contains(out1, "literal API key") {
		t.Errorf("first call log = %q, want it to mention 'literal API key'", out1)
	}
	if strings.Contains(out1, "sk-abc") {
		t.Errorf("first call log leaked the key value: %q", out1)
	}

	buf.Reset()
	got, err = ResolveAPIKey(context.Background(), repo, nil, "")
	if err != nil {
		t.Fatalf("ResolveAPIKey #2: %v", err)
	}
	if got.Value != "sk-abc" {
		t.Errorf("second call returned %q, want %q", got.Value, "sk-abc")
	}
	if buf.Len() != 0 {
		t.Errorf("second call wrote to log: %q (warn must fire once)", buf.String())
	}
}

func TestResolveAPIKey_SecretRef_LiteralEmptyErrors(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "literal:"},
	}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error for empty literal value")
	}
}

func TestResolveAPIKey_SecretRef_UnsupportedScheme(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	repo := &fakeRepo{rows: map[string]*model.APICredential{
		"anthropic|": {SecretRef: "vault://kv/something"},
	}}
	_, err := ResolveAPIKey(context.Background(), repo, nil, "")
	if err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
	if !strings.Contains(err.Error(), "unsupported secret_ref") {
		t.Errorf("err = %q, want it to mention unsupported", err.Error())
	}
}
