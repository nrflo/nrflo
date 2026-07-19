package tools_builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGrepFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"a.go":       "package main\nfunc Foo() {}\nfunc Bar() {}\n",
		"b.go":       "package main\nfunc Foo() {}\n",
		"c.txt":      "no matches here\njust text\n",
		"sub/d.go":   "package sub\nfunc Foo() {}\n",
		"sub/e.json": "{\"Foo\": 1}\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFSTools_Grep_FilesWithMatchesMode(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"func Foo","output_mode":"files_with_matches"}`)
	if isErr {
		t.Fatalf("grep = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") || strings.Contains(out, "c.txt") {
		t.Errorf("files_with_matches = %q, want a.go+b.go only", out)
	}
}

func TestFSTools_Grep_CountMode(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"func ","output_mode":"count"}`)
	if isErr {
		t.Fatalf("grep = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "a.go:2") || !strings.Contains(out, "b.go:1") {
		t.Errorf("count mode = %q, want a.go:2 and b.go:1", out)
	}
}

func TestFSTools_Grep_ContentModeLineNumbers(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"func Bar","output_mode":"content","glob":"a.go"}`)
	if isErr {
		t.Fatalf("grep = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "3\tfunc Bar() {}") {
		t.Errorf("content mode = %q, want cat -n formatted line 3", out)
	}
}

func TestFSTools_Grep_Context(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)

	out, isErr := invokeFS(t, "grep", env, `{"pattern":"func Bar","output_mode":"content","glob":"a.go","-B":1,"-A":0}`)
	if isErr {
		t.Fatalf("grep -B1 = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "2\tfunc Foo() {}") || !strings.Contains(out, "3\tfunc Bar() {}") {
		t.Errorf("-B1 = %q, want lines 2 and 3", out)
	}

	out, isErr = invokeFS(t, "grep", env, `{"pattern":"package main","output_mode":"content","glob":"a.go","-C":1}`)
	if isErr {
		t.Fatalf("grep -C1 = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "1\tpackage main") || !strings.Contains(out, "2\tfunc Foo() {}") {
		t.Errorf("-C1 = %q, want lines 1 and 2", out)
	}
}

func TestFSTools_Grep_GlobFilter(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"matches","glob":"*.txt"}`)
	if isErr {
		t.Fatalf("grep = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "c.txt") || strings.Contains(out, "a.go") {
		t.Errorf("glob-filtered grep = %q, want c.txt only", out)
	}
}

func TestFSTools_Grep_SlashlessGlobMatchesNested(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	// A slashless glob matches the basename at any depth (ripgrep rule): "*.go"
	// must include nested sub/d.go, not just workdir-root files.
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"func Foo","glob":"*.go"}`)
	if isErr {
		t.Fatalf("grep = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, filepath.ToSlash("sub/d.go")) {
		t.Errorf("slashless glob = %q, want root a.go and nested sub/d.go", out)
	}
	if strings.Contains(out, "e.json") {
		t.Errorf("slashless glob = %q, should not match sub/e.json", out)
	}
}

func TestFSTools_Grep_NoMatch(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"zzz_nomatch_zzz"}`)
	if isErr || !strings.Contains(out, "no matches") {
		t.Errorf("grep no-match = (%q, %v), want no-matches message", out, isErr)
	}
}

func TestFSTools_Grep_OutputCap(t *testing.T) {
	env := fsEnv(t)
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString("needle line here\n")
	}
	if err := os.WriteFile(filepath.Join(env.WorkDir, "big.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"needle","output_mode":"content"}`)
	if isErr {
		t.Fatalf("grep big = (%q, %v)", out, isErr)
	}
	if len(out) > grepOutputCap+64 {
		t.Errorf("grep output len = %d, want capped near %d", len(out), grepOutputCap)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("grep output = missing truncated marker (len %d)", len(out))
	}
}

func TestFSTools_Grep_InvalidOutputMode(t *testing.T) {
	env := fsEnv(t)
	writeGrepFixture(t, env.WorkDir)
	out, isErr := invokeFS(t, "grep", env, `{"pattern":"foo","output_mode":"bogus"}`)
	if !isErr || !strings.Contains(out, "output_mode must be") {
		t.Errorf("grep invalid output_mode = (%q, %v), want validation error", out, isErr)
	}
}
