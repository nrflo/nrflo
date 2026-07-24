package tools_builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

// invokeFindingsFromFile invokes findings_add_from_file directly (not via
// Builtins(), mirroring invokeFS's per-tool style) since these tests need
// fine control over env.Findings being nil vs a real service.
func invokeFindingsFromFile(t *testing.T, env apirun.ToolEnv, args string) (string, bool) {
	t.Helper()
	h := findingsAddFromFileHandler{}
	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(args))
	if err != nil {
		t.Fatalf("findings_add_from_file returned Go error: %v", err)
	}
	return out, isErr
}

func TestFindingsAddFromFile_PathResolution(t *testing.T) {
	env := fsEnv(t) // WorkDir=t.TempDir(), Findings=nil
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args string
		want string
	}{
		{"escape-relative", `{"key":"k","path":"../escape.txt"}`, "escapes"},
		{"escape-absolute", fmt.Sprintf(`{"key":"k","path":%q}`, filepath.Join(outside, "secret.txt")), "escapes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, isErr := invokeFindingsFromFile(t, env, tc.args)
			if !isErr || !strings.Contains(out, tc.want) {
				t.Errorf("findings_add_from_file(%s) = (%q, %v), want error containing %q", tc.args, out, isErr, tc.want)
			}
		})
	}

	// Symlink escape: a link inside the tree pointing outside must not pass.
	if err := os.Symlink(outside, filepath.Join(env.WorkDir, "link")); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"link/secret.txt"}`)
	if !isErr || !strings.Contains(out, "escapes") {
		t.Errorf("findings_add_from_file via symlink = (%q, %v), want escape error", out, isErr)
	}
}

func TestFindingsAddFromFile_MissingFile(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"nope.txt"}`)
	if !isErr {
		t.Errorf("findings_add_from_file missing file = (%q, %v), want isErr", out, isErr)
	}
}

func TestFindingsAddFromFile_EmptyWorkDir(t *testing.T) {
	env := apirun.ToolEnv{} // no WorkDir configured
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"a.txt"}`)
	if !isErr || !strings.Contains(out, "working directory") {
		t.Errorf("findings_add_from_file empty workdir = (%q, %v), want no-workdir error", out, isErr)
	}
}

func TestFindingsAddFromFile_Directory(t *testing.T) {
	env := fsEnv(t)
	if err := os.Mkdir(filepath.Join(env.WorkDir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"adir"}`)
	if !isErr || !strings.Contains(out, "is a directory") {
		t.Errorf("findings_add_from_file on directory = (%q, %v), want directory error", out, isErr)
	}
}

func TestFindingsAddFromFile_MissingKey(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"","path":"a.txt"}`)
	if !isErr || !strings.Contains(out, "key is required") {
		t.Errorf("findings_add_from_file missing key = (%q, %v), want key-required error", out, isErr)
	}
}

func TestFindingsAddFromFile_OverHardCap(t *testing.T) {
	env := fsEnv(t)
	path := filepath.Join(env.WorkDir, "huge.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(findingsFromFileMaxBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"huge.txt"}`)
	if !isErr || !strings.Contains(out, "exceeds") {
		t.Errorf("findings_add_from_file over hard cap = (%q, %v), want cap error", out, isErr)
	}
}

func TestFindingsAddFromFile_OverCallerMaxBytes(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "a.txt"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Well under the 256KB hard cap, but over the caller-supplied max_bytes.
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"a.txt","max_bytes":5}`)
	if !isErr || !strings.Contains(out, "exceeds") {
		t.Errorf("findings_add_from_file over caller max_bytes = (%q, %v), want cap error", out, isErr)
	}
}

// TestFindingsAddFromFile_NilFindingsAfterPassingChecks verifies the
// env.Findings==nil guard is reached (and only reached) once path/stat/cap
// checks pass — a failing path/cap check must short-circuit before it.
func TestFindingsAddFromFile_NilFindingsAfterPassingChecks(t *testing.T) {
	env := fsEnv(t) // Findings is nil
	if err := os.WriteFile(filepath.Join(env.WorkDir, "ok.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFindingsFromFile(t, env, `{"key":"k","path":"ok.txt"}`)
	if !isErr || !strings.Contains(out, "findings") {
		t.Errorf("findings_add_from_file with nil Findings = (%q, %v), want missing-service error mentioning findings", out, isErr)
	}
}

func TestFindingsAddFromFile_StoresAndReturnsHash(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.WorkDir = t.TempDir()
	content := []byte(`{"nested":"json content"}`)
	if err := os.WriteFile(filepath.Join(env.env.WorkDir, "evidence.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	out, isErr, err := invoke(t, env.env, "findings_add_from_file", `{"key":"evidence","path":"evidence.json"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var result struct {
		Key    string `json:"key"`
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	if uerr := json.Unmarshal([]byte(out), &result); uerr != nil {
		t.Fatalf("unmarshal output %q: %v", out, uerr)
	}
	if result.Key != "evidence" {
		t.Errorf("result.Key = %q, want evidence", result.Key)
	}
	if result.Bytes != len(content) {
		t.Errorf("result.Bytes = %d, want %d", result.Bytes, len(content))
	}
	wantSum := sha256.Sum256(content)
	if result.SHA256 != hex.EncodeToString(wantSum[:]) {
		t.Errorf("result.SHA256 = %q, want %q", result.SHA256, hex.EncodeToString(wantSum[:]))
	}

	// The finding was actually persisted with the raw file content.
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"nested":"json content"`) {
		t.Errorf("findings = %s, want persisted evidence.json content", got)
	}

	// A findings.updated event fired, mirroring findings_add's broadcast shape.
	if len(env.hub.events) != 1 {
		t.Fatalf("hub events = %d, want 1", len(env.hub.events))
	}
	if env.hub.events[0].Data["action"] != "add-from-file" {
		t.Errorf("event action = %v, want add-from-file", env.hub.events[0].Data["action"])
	}
}
