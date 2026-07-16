package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

func fsEnv(t *testing.T) apirun.ToolEnv {
	t.Helper()
	return apirun.ToolEnv{WorkDir: t.TempDir()}
}

func invokeFS(t *testing.T, name string, env apirun.ToolEnv, args string) (string, bool) {
	t.Helper()
	h, ok := FSTools()[name]
	if !ok {
		t.Fatalf("no fs tool %q", name)
	}
	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s returned Go error: %v", name, err)
	}
	return out, isErr
}

func TestFSTools_WorkdirJail(t *testing.T) {
	env := fsEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		`{"path":"../escape.txt","old_string":"","new_string":"x"}`,
		fmt.Sprintf(`{"path":%q,"old_string":"","new_string":"x"}`, filepath.Join(outside, "e.txt")),
	}
	for _, args := range cases {
		if out, isErr := invokeFS(t, "edit_file", env, args); !isErr || !strings.Contains(out, "escapes") {
			t.Errorf("edit_file(%s) = (%q, %v), want escape error", args, out, isErr)
		}
	}
	if out, isErr := invokeFS(t, "read_file", env, fmt.Sprintf(`{"path":%q}`, filepath.Join(outside, "secret.txt"))); !isErr || !strings.Contains(out, "escapes") {
		t.Errorf("read_file outside jail = (%q, %v), want escape error", out, isErr)
	}

	// Symlink escape: a link inside the tree pointing outside must not pass.
	if err := os.Symlink(outside, filepath.Join(env.WorkDir, "link")); err != nil {
		t.Fatal(err)
	}
	if out, isErr := invokeFS(t, "read_file", env, `{"path":"link/secret.txt"}`); !isErr || !strings.Contains(out, "escapes") {
		t.Errorf("read_file via symlink = (%q, %v), want escape error", out, isErr)
	}

	// No workdir at all → error.
	if out, isErr := invokeFS(t, "bash", apirun.ToolEnv{}, `{"command":"true"}`); !isErr || !strings.Contains(out, "working directory") {
		t.Errorf("bash without workdir = (%q, %v), want no-workdir error", out, isErr)
	}
}

func TestFSTools_EditCreateReadRoundtrip(t *testing.T) {
	env := fsEnv(t)

	if out, isErr := invokeFS(t, "edit_file", env, `{"path":"sub/a.txt","old_string":"","new_string":"one\ntwo\nthree"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}
	if out, isErr := invokeFS(t, "edit_file", env, `{"path":"sub/a.txt","old_string":"","new_string":"x"}`); !isErr || !strings.Contains(out, "already exists") {
		t.Errorf("re-create = (%q, %v), want already-exists error", out, isErr)
	}

	out, isErr := invokeFS(t, "read_file", env, `{"path":"sub/a.txt","offset":2,"limit":1}`)
	if isErr || !strings.Contains(out, "2\ttwo") || strings.Contains(out, "one") {
		t.Errorf("read offset/limit = (%q, %v), want only line 2", out, isErr)
	}

	if out, isErr := invokeFS(t, "edit_file", env, `{"path":"sub/a.txt","old_string":"nope","new_string":"x"}`); !isErr || !strings.Contains(out, "not found") {
		t.Errorf("missing old_string = (%q, %v)", out, isErr)
	}
	if out, isErr := invokeFS(t, "edit_file", env, `{"path":"sub/a.txt","old_string":"t","new_string":"T"}`); !isErr || !strings.Contains(out, "matches 2 times") {
		t.Errorf("ambiguous old_string = (%q, %v), want multi-match error", out, isErr)
	}
	if out, isErr := invokeFS(t, "edit_file", env, `{"path":"sub/a.txt","old_string":"t","new_string":"T","replace_all":true}`); isErr || !strings.Contains(out, "2 replacement") {
		t.Errorf("replace_all = (%q, %v)", out, isErr)
	}

	data, err := os.ReadFile(filepath.Join(env.WorkDir, "sub", "a.txt"))
	if err != nil || string(data) != "one\nTwo\nThree" {
		t.Errorf("final content = %q err=%v, want one/Two/Three", data, err)
	}
}

func TestFSTools_Bash(t *testing.T) {
	env := fsEnv(t)

	out, isErr := invokeFS(t, "bash", env, `{"command":"pwd && echo hi"}`)
	if isErr || !strings.Contains(out, "hi") {
		t.Fatalf("bash echo = (%q, %v)", out, isErr)
	}
	resolved, _ := filepath.EvalSymlinks(env.WorkDir)
	if !strings.Contains(out, resolved) {
		t.Errorf("bash cwd output %q does not contain workdir %q", out, resolved)
	}

	if out, isErr := invokeFS(t, "bash", env, `{"command":"echo boom >&2; exit 3"}`); !isErr || !strings.Contains(out, "boom") {
		t.Errorf("bash non-zero exit = (%q, %v), want isErr with stderr", out, isErr)
	}
}
