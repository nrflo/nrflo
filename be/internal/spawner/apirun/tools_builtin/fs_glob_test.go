package tools_builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeGlobFixture(t *testing.T, root string) {
	t.Helper()
	files := []string{
		"a.go",
		"b.txt",
		filepath.Join("sub", "c.go"),
		filepath.Join("sub", "deeper", "d.go"),
		filepath.Join("sub", "e.txt"),
	}
	for _, rel := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFSTools_Glob_PatternMatch(t *testing.T) {
	env := fsEnv(t)
	writeGlobFixture(t, env.WorkDir)

	out, isErr := invokeFS(t, "glob", env, `{"pattern":"**/*.go"}`)
	if isErr {
		t.Fatalf("glob = (%q, %v)", out, isErr)
	}
	for _, want := range []string{"a.go", "sub/c.go", "sub/deeper/d.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("glob **/*.go missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "b.txt") || strings.Contains(out, "e.txt") {
		t.Errorf("glob **/*.go matched a .txt file:\n%s", out)
	}
}

func TestFSTools_Glob_MtimeDescOrdering(t *testing.T) {
	env := fsEnv(t)
	writeGlobFixture(t, env.WorkDir)

	base := time.Now()
	times := map[string]time.Time{
		"a.go":            base.Add(-3 * time.Hour),
		"sub/c.go":        base.Add(-2 * time.Hour),
		"sub/deeper/d.go": base.Add(-1 * time.Hour),
	}
	for rel, mt := range times {
		p := filepath.Join(env.WorkDir, rel)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}

	out, isErr := invokeFS(t, "glob", env, `{"pattern":"**/*.go"}`)
	if isErr {
		t.Fatalf("glob = (%q, %v)", out, isErr)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"sub/deeper/d.go", "sub/c.go", "a.go"}
	if len(lines) != len(want) {
		t.Fatalf("glob lines = %v, want %v", lines, want)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("glob order[%d] = %q, want %q (full: %v)", i, lines[i], w, lines)
		}
	}
}

func TestFSTools_Glob_JailConfinement(t *testing.T) {
	env := fsEnv(t)
	writeGlobFixture(t, env.WorkDir)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(env.WorkDir, "link")); err != nil {
		t.Fatal(err)
	}

	out, isErr := invokeFS(t, "glob", env, `{"pattern":"**/*.go"}`)
	if isErr {
		t.Fatalf("glob = (%q, %v)", out, isErr)
	}
	if strings.Contains(out, "secret.go") {
		t.Errorf("glob leaked outside the jail via symlink:\n%s", out)
	}
}

func TestFSTools_Glob_NoMatch(t *testing.T) {
	env := fsEnv(t)
	writeGlobFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "glob", env, `{"pattern":"**/*.rs"}`)
	if isErr || !strings.Contains(out, "no files matched") {
		t.Errorf("glob no-match = (%q, %v), want no-files-matched message", out, isErr)
	}
}

func TestFSTools_Glob_MissingPattern(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "glob", env, `{"pattern":""}`)
	if !isErr || !strings.Contains(out, "pattern is required") {
		t.Errorf("glob empty pattern = (%q, %v), want pattern-required error", out, isErr)
	}
}
