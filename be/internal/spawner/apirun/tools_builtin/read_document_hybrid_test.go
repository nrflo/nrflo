package tools_builtin

// Tests for ReadDocumentHybridHandler — the cli_interactive variant for CLIs
// without native document reading (codex): images come back as media blocks
// (attached over the MCP bridge), PDFs come back as a path to process on disk.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func addHybridArtifact(t *testing.T, env *builtinTestEnv, name, content, contentType string) {
	t.Helper()
	addInput, _ := json.Marshal(map[string]string{
		"name":         name,
		"content":      content,
		"content_type": contentType,
	})
	out, isErr, err := invoke(t, env.env, "artifact_add", string(addInput))
	if err != nil || isErr {
		t.Fatalf("artifact_add failed: err=%v isErr=%v out=%q", err, isErr, out)
	}
}

func TestReadDocumentHybridHandler_ImageReturnsMedia(t *testing.T) {
	env := newBuiltinTestEnv(t)
	content := "png-bytes-stand-in"
	addHybridArtifact(t, env, "scan.png", content, "image/png")

	out, media, isErr, err := ReadDocumentHybridHandler{}.InvokeMedia(
		context.Background(), env.env, json.RawMessage(`{"name":"scan.png"}`))
	if err != nil || isErr {
		t.Fatalf("InvokeMedia failed: err=%v isErr=%v out=%q", err, isErr, out)
	}
	if len(media) != 1 {
		t.Fatalf("media len = %d, want 1", len(media))
	}
	if media[0].Kind != "image" || media[0].MediaType != "image/png" || media[0].Name != "scan.png" {
		t.Errorf("media[0] = %+v", media[0])
	}
	if media[0].DataB64 != base64.StdEncoding.EncodeToString([]byte(content)) {
		t.Errorf("DataB64 mismatch")
	}
	if !strings.Contains(out, "scan.png") || !strings.Contains(out, "/") {
		t.Errorf("output should name the file and its path; out=%q", out)
	}
}

func TestReadDocumentHybridHandler_PDFReturnsPathOnly(t *testing.T) {
	env := newBuiltinTestEnv(t)
	addHybridArtifact(t, env, "deed.pdf", "%PDF-1.4 fake", "application/pdf")

	out, media, isErr, err := ReadDocumentHybridHandler{}.InvokeMedia(
		context.Background(), env.env, json.RawMessage(`{"name":"deed.pdf"}`))
	if err != nil || isErr {
		t.Fatalf("InvokeMedia failed: err=%v isErr=%v out=%q", err, isErr, out)
	}
	if len(media) != 0 {
		t.Fatalf("media len = %d, want 0 for PDF", len(media))
	}
	var result struct {
		Path string `json:"path"`
		Note string `json:"note"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
		t.Fatalf("unmarshal result: %v, raw=%q", jsonErr, out)
	}
	if result.Path == "" || result.Note == "" {
		t.Errorf("want path + note, got %+v", result)
	}
}

func TestReadDocumentHybridHandler_SpecName(t *testing.T) {
	if got := (ReadDocumentHybridHandler{}).Spec().Name; got != "read_document" {
		t.Errorf("Spec().Name = %q, want read_document", got)
	}
}

func TestReadDocumentHybridHandler_NotFoundIsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, media, isErr, err := ReadDocumentHybridHandler{}.InvokeMedia(
		context.Background(), env.env, json.RawMessage(`{"name":"missing.png"}`))
	if err != nil {
		t.Fatalf("InvokeMedia err: %v", err)
	}
	if !isErr || len(media) != 0 {
		t.Errorf("want isErr=true and no media, got isErr=%v media=%d out=%q", isErr, len(media), out)
	}
}

func TestReadDocumentHybridHandler_NotInBuiltins(t *testing.T) {
	if h, ok := Builtins()["read_document"]; ok {
		if _, isHybrid := h.(ReadDocumentHybridHandler); isHybrid {
			t.Error("Builtins()[read_document] is ReadDocumentHybridHandler; it must not be registered there")
		}
	}
}
