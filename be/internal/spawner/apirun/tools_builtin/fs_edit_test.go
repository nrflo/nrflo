package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

func TestFSTools_Edit_ReadBeforeEditRejection(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"hello world"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}

	// A fresh session that never read the file must be rejected — even
	// though the file exists on disk (written by a different session here).
	freshEnv := env
	freshEnv.FS = apirun.NewFSSession()
	out, isErr := invokeFS(t, "edit_file", freshEnv, `{"path":"a.txt","old_string":"hello","new_string":"bye"}`)
	if !isErr || !strings.Contains(out, "read_file it first") {
		t.Errorf("edit without read = (%q, %v), want read-before-edit rejection", out, isErr)
	}
}

func TestFSTools_Edit_NoOpRejection(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"hello"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", env, `{"path":"a.txt","old_string":"hello","new_string":"hello"}`)
	if !isErr || !strings.Contains(out, "identical") {
		t.Errorf("no-op edit = (%q, %v), want identical-strings rejection", out, isErr)
	}
}

func TestFSTools_Edit_UniquenessViolationWithMatchCount(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"aa aa aa"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", env, `{"path":"a.txt","old_string":"aa","new_string":"bb"}`)
	if !isErr || !strings.Contains(out, "matches 3 times") {
		t.Errorf("ambiguous edit = (%q, %v), want match-count error", out, isErr)
	}
}

func TestFSTools_Edit_ReplaceAllSuccess(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"aa aa aa"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", env, `{"path":"a.txt","old_string":"aa","new_string":"bb","replace_all":true}`)
	if isErr || !strings.Contains(out, "3 replacement") {
		t.Errorf("replace_all = (%q, %v), want 3 replacements", out, isErr)
	}
}

func TestFSTools_Edit_AfterReadFileSucceeds(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "write_file", env, `{"path":"a.txt","content":"hello"}`); isErr {
		t.Fatalf("create = (%q, %v)", out, isErr)
	}

	// A fresh session must read_file before edit_file will accept it.
	freshEnv := env
	freshEnv.FS = apirun.NewFSSession()
	if out, isErr := invokeFS(t, "read_file", freshEnv, `{"path":"a.txt"}`); isErr {
		t.Fatalf("read_file = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", freshEnv, `{"path":"a.txt","old_string":"hello","new_string":"bye"}`)
	if isErr || !strings.Contains(out, "edited a.txt") {
		t.Errorf("edit after read_file = (%q, %v), want success", out, isErr)
	}
}
