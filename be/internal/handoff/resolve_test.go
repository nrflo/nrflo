package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkFile creates a file at root/relPath, making parent dirs as needed.
func mkFile(t *testing.T, root, relPath string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestResolvePaths_AbsoluteInsideRoot(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "pkg/foo.go")
	// t.TempDir() can itself be a symlink (e.g. macOS /var -> /private/var);
	// resolve it the same way resolvePaths resolves root before building an
	// absolute candidate path, or the containment check spuriously fails.
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", root, err)
	}

	verified, unverified := resolvePaths(root, []string{filepath.Join(evalRoot, "pkg/foo.go")})
	if len(unverified) != 0 {
		t.Errorf("unverified = %v, want empty", unverified)
	}
	if len(verified) != 1 || verified[0] != filepath.Join("pkg", "foo.go") {
		t.Errorf("verified = %v, want [pkg/foo.go]", verified)
	}
}

func TestResolvePaths_RelativeHit(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "pkg/foo.go")

	verified, unverified := resolvePaths(root, []string{"pkg/foo.go"})
	if len(unverified) != 0 {
		t.Errorf("unverified = %v, want empty", unverified)
	}
	if len(verified) != 1 || verified[0] != filepath.Join("pkg", "foo.go") {
		t.Errorf("verified = %v, want [pkg/foo.go]", verified)
	}
}

func TestResolvePaths_AbsoluteOutsideRoot_Rejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mkFile(t, outside, "secret.go")

	verified, unverified := resolvePaths(root, []string{filepath.Join(outside, "secret.go")})
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty (outside root must never verify)", verified)
	}
	if len(unverified) != 1 {
		t.Fatalf("unverified = %v, want 1 entry", unverified)
	}
}

func TestResolvePaths_UniqueBasename_Canonicalized(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "src/components/Widget.tsx")

	verified, unverified := resolvePaths(root, []string{"Widget.tsx"})
	if len(unverified) != 0 {
		t.Errorf("unverified = %v, want empty", unverified)
	}
	if len(verified) != 1 || verified[0] != filepath.Join("src", "components", "Widget.tsx") {
		t.Errorf("verified = %v, want [src/components/Widget.tsx]", verified)
	}
}

func TestResolvePaths_AmbiguousBasename_StaysUnverified(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "a/dup.go")
	mkFile(t, root, "b/dup.go")

	verified, unverified := resolvePaths(root, []string{"dup.go"})
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty for ambiguous basename", verified)
	}
	if len(unverified) != 1 || unverified[0] != "dup.go" {
		t.Errorf("unverified = %v, want [dup.go] verbatim", unverified)
	}
}

func TestResolvePaths_NonexistentPath_StaysUnverified(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "pkg/foo.go")

	verified, unverified := resolvePaths(root, []string{"pkg/missing.go"})
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty", verified)
	}
	if len(unverified) != 1 || unverified[0] != "pkg/missing.go" {
		t.Errorf("unverified = %v, want [pkg/missing.go]", unverified)
	}
}

// TestResolvePaths_ExtensionMismatch_NeverFuzzyMatches is the epic
// regression: only LogMessage.tsx exists on disk, but the agent referenced
// LogMessage.ts. It must land in unverified, verbatim, and the resolved
// output must never contain "LogMessage.tsx" for that candidate.
func TestResolvePaths_ExtensionMismatch_NeverFuzzyMatches(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "src/components/LogMessage.tsx")

	verified, unverified := resolvePaths(root, []string{"LogMessage.ts"})
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty — must not fuzzy-match .ts to .tsx", verified)
	}
	if len(unverified) != 1 || unverified[0] != "LogMessage.ts" {
		t.Errorf("unverified = %v, want [LogMessage.ts] verbatim", unverified)
	}
	for _, v := range verified {
		if strings.Contains(v, "LogMessage.tsx") {
			t.Errorf("verified output leaked LogMessage.tsx for a .ts candidate: %v", verified)
		}
	}
}

func TestResolvePaths_EmptyRoot_EveryCandidateUnverified(t *testing.T) {
	cands := []string{"pkg/foo.go", "/abs/path.go", "bare.go"}
	verified, unverified := resolvePaths("", cands)
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty when root is empty", verified)
	}
	if len(unverified) != len(cands) {
		t.Errorf("unverified = %v, want all %d candidates", unverified, len(cands))
	}
}

func TestResolvePaths_MissingRoot_EveryCandidateUnverified_NoPanic(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cands := []string{"pkg/foo.go", "bare.go"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("resolvePaths panicked on missing root: %v", r)
		}
	}()

	verified, unverified := resolvePaths(missing, cands)
	if len(verified) != 0 {
		t.Errorf("verified = %v, want empty for missing root", verified)
	}
	if len(unverified) != len(cands) {
		t.Errorf("unverified = %v, want all candidates unverified", unverified)
	}
}

func TestResolvePaths_NoCandidates(t *testing.T) {
	root := t.TempDir()
	verified, unverified := resolvePaths(root, nil)
	if len(verified) != 0 || len(unverified) != 0 {
		t.Errorf("verified=%v unverified=%v, want both empty", verified, unverified)
	}
}
