package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecretRef_Env(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		t.Setenv("TEST_SR_PRESENT_7f3a", "myvalue")
		got, err := ResolveSecretRef("env:TEST_SR_PRESENT_7f3a")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if got != "myvalue" {
			t.Errorf("got %q, want myvalue", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		t.Setenv("TEST_SR_EMPTY_7f3a", "")
		if _, err := ResolveSecretRef("env:TEST_SR_EMPTY_7f3a"); err == nil {
			t.Error("expected error for empty env var, got nil")
		}
	})
	t.Run("unset", func(t *testing.T) {
		os.Unsetenv("TEST_SR_UNSET_7f3a")
		if _, err := ResolveSecretRef("env:TEST_SR_UNSET_7f3a"); err == nil {
			t.Error("expected error for unset env var, got nil")
		}
	})
	t.Run("empty_name", func(t *testing.T) {
		if _, err := ResolveSecretRef("env:"); err == nil {
			t.Error("expected error for empty env name, got nil")
		}
	})
}

func TestResolveSecretRef_Literal(t *testing.T) {
	t.Parallel()
	t.Run("present", func(t *testing.T) {
		got, err := ResolveSecretRef("literal:supersecret")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if got != "supersecret" {
			t.Errorf("got %q, want supersecret", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := ResolveSecretRef("literal:"); err == nil {
			t.Error("expected error for empty literal value, got nil")
		}
	})
}

func TestResolveSecretRef_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("present", func(t *testing.T) {
		path := filepath.Join(dir, "secret.txt")
		if err := os.WriteFile(path, []byte("filesecret\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ResolveSecretRef("file:" + path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if got != "filesecret" {
			t.Errorf("got %q, want filesecret (TrimSpace applied)", got)
		}
	})
	t.Run("empty_content", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := ResolveSecretRef("file:" + path); err == nil {
			t.Error("expected error for whitespace-only file content, got nil")
		}
	})
	t.Run("missing", func(t *testing.T) {
		if _, err := ResolveSecretRef("file:" + filepath.Join(dir, "nofile.txt")); err == nil {
			t.Error("expected error for missing file, got nil")
		}
	})
	t.Run("non_readable", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root, permission check skipped")
		}
		path := filepath.Join(dir, "noread.txt")
		if err := os.WriteFile(path, []byte("secret"), 0o000); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := ResolveSecretRef("file:" + path); err == nil {
			t.Error("expected error for non-readable file, got nil")
		}
	})
	t.Run("empty_path", func(t *testing.T) {
		if _, err := ResolveSecretRef("file:"); err == nil {
			t.Error("expected error for empty file path, got nil")
		}
	})
}

func TestResolveSecretRef_Unknown(t *testing.T) {
	t.Parallel()
	if _, err := ResolveSecretRef("unknown:something"); err == nil {
		t.Error("expected error for unknown scheme, got nil")
	}
}

func TestRedactSecretRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref  string
		want string
	}{
		{"literal:supersecret", "literal:***"},
		{"literal:anything", "literal:***"},
		{"literal:", "literal:***"},
		{"env:MY_VAR", "env:MY_VAR"},
		{"file:/path/to/secret", "file:/path/to/secret"},
		{"unknown:foo", "unknown:foo"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.ref, func(t *testing.T) {
			got := RedactSecretRef(tc.ref)
			if got != tc.want {
				t.Errorf("RedactSecretRef(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}
