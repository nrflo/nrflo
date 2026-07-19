package tools_builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSTools_Write_CreateNewFile(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "write_file", env, `{"path":"new.txt","content":"hello"}`)
	if isErr || !strings.Contains(out, "wrote new.txt") {
		t.Fatalf("write_file create = (%q, %v)", out, isErr)
	}
	data, err := os.ReadFile(filepath.Join(env.WorkDir, "new.txt"))
	if err != nil || string(data) != "hello" {
		t.Errorf("file content = %q err=%v, want %q", data, err, "hello")
	}
}

func TestFSTools_Write_RefuseOverwriteUnread(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "existing.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "write_file", env, `{"path":"existing.txt","content":"clobbered"}`)
	if !isErr || !strings.Contains(out, "read it first") {
		t.Errorf("write_file unread overwrite = (%q, %v), want refusal", out, isErr)
	}
	data, err := os.ReadFile(filepath.Join(env.WorkDir, "existing.txt"))
	if err != nil || string(data) != "original" {
		t.Errorf("file was modified despite refusal: %q err=%v", data, err)
	}
}

func TestFSTools_Write_AllowOverwriteAfterRead(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "existing.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, isErr := invokeFS(t, "read_file", env, `{"path":"existing.txt"}`); isErr {
		t.Fatalf("read_file = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "write_file", env, `{"path":"existing.txt","content":"clobbered"}`)
	if isErr {
		t.Fatalf("write_file after read = (%q, %v), want success", out, isErr)
	}
	data, err := os.ReadFile(filepath.Join(env.WorkDir, "existing.txt"))
	if err != nil || string(data) != "clobbered" {
		t.Errorf("file content = %q err=%v, want %q", data, err, "clobbered")
	}
}

func TestFSTools_Write_NilFSSkipsReadCheck(t *testing.T) {
	env := fsEnv(t)
	env.FS = nil
	if err := os.WriteFile(filepath.Join(env.WorkDir, "existing.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "write_file", env, `{"path":"existing.txt","content":"clobbered"}`)
	if isErr {
		t.Errorf("write_file with nil FS = (%q, %v), want unconditional overwrite", out, isErr)
	}
}

func TestFSTools_Write_ParentDirCreation(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "write_file", env, `{"path":"a/b/c/deep.txt","content":"nested"}`)
	if isErr {
		t.Fatalf("write_file nested = (%q, %v)", out, isErr)
	}
	data, err := os.ReadFile(filepath.Join(env.WorkDir, "a", "b", "c", "deep.txt"))
	if err != nil || string(data) != "nested" {
		t.Errorf("nested file content = %q err=%v", data, err)
	}
}

func TestFSTools_Write_JailEscape(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "write_file", env, `{"path":"../escape.txt","content":"x"}`)
	if !isErr || !strings.Contains(out, "escapes") {
		t.Errorf("write_file escape = (%q, %v), want escape error", out, isErr)
	}
}

func TestFSTools_Write_MarksReadForFollowupEdit(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"one"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}
	// Newly-written files are immediately eligible for edit_file without a
	// separate read_file call.
	out, isErr := invokeFS(t, "edit_file", env, `{"path":"a.txt","old_string":"one","new_string":"two"}`)
	if isErr {
		t.Errorf("edit after write_file = (%q, %v), want success (write_file marks read)", out, isErr)
	}
}
