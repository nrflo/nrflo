package tools_builtin

// Tests for ReadDocumentPathHandler — the api-via-cli variant of read_document
// that returns {"path": absPath} instead of inlining the document bytes.
// NOT registered in Builtins(); the hybrid prep swaps it in at spawn time.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestReadDocumentPathHandler_ReturnsPath verifies that invoking the handler
// with an existing artifact materializes it to disk and returns the absolute
// path in the JSON output; the file's content must match what was uploaded.
func TestReadDocumentPathHandler_ReturnsPath(t *testing.T) {
	env := newBuiltinTestEnv(t)

	content := "hello from the path handler test"
	addInput, _ := json.Marshal(map[string]string{
		"name":         "doc.txt",
		"content":      content,
		"content_type": "text/plain",
	})
	out, isErr, err := invoke(t, env.env, "artifact_add", string(addInput))
	if err != nil {
		t.Fatalf("artifact_add Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("artifact_add isErr=true, output=%q", out)
	}

	readInput, _ := json.Marshal(map[string]string{"name": "doc.txt"})
	out, isErr, err = ReadDocumentPathHandler{}.Invoke(context.Background(), env.env, json.RawMessage(readInput))
	if err != nil {
		t.Fatalf("ReadDocumentPathHandler.Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, output=%q; want success", out)
	}

	var result struct {
		Path string `json:"path"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
		t.Fatalf("unmarshal result: %v, raw=%q", jsonErr, out)
	}
	if result.Path == "" {
		t.Fatal("path is empty in result; want absolute path to materialized artifact")
	}

	b, readErr := os.ReadFile(result.Path)
	if readErr != nil {
		t.Fatalf("ReadFile(%q): %v", result.Path, readErr)
	}
	if string(b) != content {
		t.Errorf("file content = %q, want %q", string(b), content)
	}
}

// TestReadDocumentPathHandler_SpecName verifies that Spec().Name is "read_document"
// (same name as the media variant) so it replaces the tool in the registry.
func TestReadDocumentPathHandler_SpecName(t *testing.T) {
	spec := ReadDocumentPathHandler{}.Spec()
	if spec.Name != "read_document" {
		t.Errorf("Spec().Name = %q, want read_document", spec.Name)
	}
}

// TestReadDocumentPathHandler_SpecDescriptionMentionsPath verifies that the
// description communicates that a path is returned (not inline content), so the
// model knows to use its native Read tool.
func TestReadDocumentPathHandler_SpecDescriptionMentionsPath(t *testing.T) {
	spec := ReadDocumentPathHandler{}.Spec()
	if !strings.Contains(spec.Description, "path") {
		t.Errorf("Spec().Description = %q; want to mention path", spec.Description)
	}
}

// TestReadDocumentPathHandler_NotFoundIsError verifies that requesting an artifact
// that does not exist returns isErr=true.
func TestReadDocumentPathHandler_NotFoundIsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	input, _ := json.Marshal(map[string]string{"name": "nonexistent.pdf"})
	out, isErr, err := ReadDocumentPathHandler{}.Invoke(context.Background(), env.env, json.RawMessage(input))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, output=%q; want isErr=true for missing artifact", out)
	}
}

// TestReadDocumentPathHandler_EmptyNameIsError verifies that an empty name field
// returns isErr=true.
func TestReadDocumentPathHandler_EmptyNameIsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	input := json.RawMessage(`{"name":""}`)
	out, isErr, err := ReadDocumentPathHandler{}.Invoke(context.Background(), env.env, json.RawMessage(input))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, output=%q; want isErr=true for empty name", out)
	}
}

// TestReadDocumentPathHandler_NotInBuiltins verifies that Builtins()["read_document"]
// is the media variant (readDocumentHandler), not ReadDocumentPathHandler.
func TestReadDocumentPathHandler_NotInBuiltins(t *testing.T) {
	if h, ok := Builtins()["read_document"]; ok {
		if _, isPath := h.(ReadDocumentPathHandler); isPath {
			t.Error("Builtins()[read_document] is ReadDocumentPathHandler; it must not be registered there")
		}
	}
}
