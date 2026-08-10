package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func setupProjectCwdEnv(t *testing.T) *ProjectService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "project_cwd_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewProjectService(pool, clock.NewTest(time.Now().UTC()))
}

func insertProjectRoot(t *testing.T, svc *ProjectService, id, root string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := svc.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?,?,?,?,?)`,
		id, id, root, now, now,
	); err != nil {
		t.Fatalf("insert project %s: %v", id, err)
	}
}

func TestResolveByCwd(t *testing.T) {
	svc := setupProjectCwdEnv(t)

	base := t.TempDir()
	foo := filepath.Join(base, "foo")
	foobar := filepath.Join(base, "foobar")
	for _, d := range []string{filepath.Join(foo, "sub"), foobar} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	insertProjectRoot(t, svc, "foo", foo)
	insertProjectRoot(t, svc, "foobar", foobar)

	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"exact root", foo, "foo"},
		{"nested under root", filepath.Join(foo, "sub"), "foo"},
		{"segment boundary (foobar not under foo)", foobar, "foobar"},
		{"no match", base, ""},
		{"empty cwd", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ResolveByCwd(tc.cwd)
			if err != nil {
				t.Fatalf("ResolveByCwd: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveByCwd(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}

// TestResolveByCwd_CaseFold verifies a cwd whose case differs from the
// registered root still matches when path comparison is case-insensitive
// (darwin default APFS), and does not match when it is case-sensitive.
func TestResolveByCwd_CaseFold(t *testing.T) {
	svc := setupProjectCwdEnv(t)
	// Pre-resolve symlinks (macOS /var → /private/var) so the nonexistent
	// lowercase variant canonicalizes consistently with the real root.
	base := canonProjectPath(t.TempDir())
	root := filepath.Join(base, "KDRE")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	insertProjectRoot(t, svc, "kdre", root)
	lower := filepath.Join(base, "kdre")

	orig := pathCaseInsensitive
	t.Cleanup(func() { pathCaseInsensitive = orig })

	pathCaseInsensitive = true
	got, err := svc.ResolveByCwd(lower)
	if err != nil {
		t.Fatalf("ResolveByCwd: %v", err)
	}
	if got != "kdre" {
		t.Errorf("case-insensitive ResolveByCwd(%q) = %q, want kdre", lower, got)
	}

	pathCaseInsensitive = false
	got, err = svc.ResolveByCwd(lower)
	if err != nil {
		t.Fatalf("ResolveByCwd: %v", err)
	}
	if got != "" {
		t.Errorf("case-sensitive ResolveByCwd(%q) = %q, want empty", lower, got)
	}
}

// TestResolveByCwd_RootPathNotCatchAll verifies a "/" root_path never becomes a
// catch-all match.
func TestResolveByCwd_RootPathNotCatchAll(t *testing.T) {
	svc := setupProjectCwdEnv(t)
	insertProjectRoot(t, svc, "rooted", string(os.PathSeparator))

	got, err := svc.ResolveByCwd(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveByCwd: %v", err)
	}
	if got != "" {
		t.Errorf("ResolveByCwd = %q, want empty (root path must not match)", got)
	}
}
