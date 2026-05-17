package tools_builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"be/internal/ws"
)

func TestArtifactAdd_UTF8Content(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"hello.txt","content":"héllo wörld","content_type":"text/plain"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, raw=%q", err, out)
	}
	if resp["name"] != "hello.txt" {
		t.Errorf("name = %q, want hello.txt", resp["name"])
	}
	if resp["id"] == "" {
		t.Error("id is empty")
	}

	artifacts, err := env.env.ArtifactSvc.List(context.Background(), testWFIID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("List count = %d, want 1", len(artifacts))
	}
	if artifacts[0].Name != "hello.txt" {
		t.Errorf("artifact name = %q, want hello.txt", artifacts[0].Name)
	}
}

func TestArtifactAdd_Base64Content(t *testing.T) {
	env := newBuiltinTestEnv(t)
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	b64 := base64.StdEncoding.EncodeToString(data)
	input, _ := json.Marshal(map[string]string{
		"name":         "img.png",
		"content_b64":  b64,
		"content_type": "image/png",
	})

	out, isErr, err := invoke(t, env.env, "artifact_add", string(input))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	artifacts, err := env.env.ArtifactSvc.List(context.Background(), testWFIID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("List count = %d, want 1", len(artifacts))
	}
	if artifacts[0].SizeBytes != int64(len(data)) {
		t.Errorf("size_bytes = %d, want %d", artifacts[0].SizeBytes, len(data))
	}
	if artifacts[0].ContentType != "image/png" {
		t.Errorf("content_type = %q, want image/png", artifacts[0].ContentType)
	}
}

func TestArtifactAdd_BothContentAndB64_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"f.txt","content":"hello","content_b64":"aGVsbG8="}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for mutually exclusive fields")
	}
	if !strings.Contains(out, "mutually exclusive") {
		t.Errorf("output=%q, want 'mutually exclusive'", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no events on validation error, got %d", len(env.hub.events))
	}
}

func TestArtifactAdd_TooLarge_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	// 32 MiB + 1 byte base64-encoded
	bigData := make([]byte, 32*1024*1024+1)
	b64 := base64.StdEncoding.EncodeToString(bigData)
	input, _ := json.Marshal(map[string]string{
		"name":        "big.bin",
		"content_b64": b64,
	})

	out, isErr, err := invoke(t, env.env, "artifact_add", string(input))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for oversized payload")
	}
	if !strings.Contains(out, "max 32 MiB") {
		t.Errorf("output=%q, want 'max 32 MiB'", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no events on size error, got %d", len(env.hub.events))
	}
}

func TestArtifactAdd_BadBase64_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"f.bin","content_b64":"not!valid!base64!!!"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for bad base64")
	}
	if !strings.Contains(strings.ToLower(out), "base64") {
		t.Errorf("output=%q, want base64 mention", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no events on bad base64, got %d", len(env.hub.events))
	}
}

func TestArtifactAdd_EmitsArtifactCreatedEvents(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"ev.txt","content":"data"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatal("isErr=true unexpectedly")
	}

	// AddFromAgent broadcasts once internally; the handler broadcasts a second time.
	if len(env.hub.events) == 0 {
		t.Fatal("hub event count = 0, want at least 1")
	}
	for i, ev := range env.hub.events {
		if ev.Type != ws.EventArtifactCreated {
			t.Errorf("event[%d] type = %q, want %q", i, ev.Type, ws.EventArtifactCreated)
		}
		if ev.Data["workflow_instance_id"] != testWFIID {
			t.Errorf("event[%d] workflow_instance_id = %v, want %q", i, ev.Data["workflow_instance_id"], testWFIID)
		}
		if ev.Data["name"] != "ev.txt" {
			t.Errorf("event[%d] name = %v, want ev.txt", i, ev.Data["name"])
		}
	}
}

func TestArtifactList_ReturnsAddedRows(t *testing.T) {
	env := newBuiltinTestEnv(t)

	if _, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"a.txt","content":"aaa","content_type":"text/plain"}`); err != nil || isErr {
		t.Fatalf("add a.txt: isErr=%v err=%v", isErr, err)
	}
	if _, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"b.txt","content":"bb"}`); err != nil || isErr {
		t.Fatalf("add b.txt: isErr=%v err=%v", isErr, err)
	}

	out, isErr, err := invoke(t, env.env, "artifact_list", `{}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("unmarshal list: %v, raw=%q", err, out)
	}
	if len(items) != 2 {
		t.Fatalf("list count = %d, want 2", len(items))
	}

	names := map[string]bool{}
	for _, item := range items {
		name, _ := item["name"].(string)
		names[name] = true
		if item["size_bytes"] == nil {
			t.Errorf("item %q missing size_bytes", name)
		}
		if item["source"] == nil {
			t.Errorf("item %q missing source", name)
		}
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("list names = %v, want a.txt and b.txt", names)
	}
}

func TestArtifactGet_MaterializesToDisk(t *testing.T) {
	env := newBuiltinTestEnv(t)
	content := "artifact content here"

	if _, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"get.txt","content":"`+content+`"}`); err != nil || isErr {
		t.Fatalf("add artifact: isErr=%v err=%v", isErr, err)
	}

	out, isErr, err := invoke(t, env.env, "artifact_get", `{"name":"get.txt"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var resp map[string]string
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, raw=%q", err, out)
	}
	absPath := resp["path"]
	if absPath == "" {
		t.Fatal("path is empty")
	}

	diskBytes, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", absPath, err)
	}
	if string(diskBytes) != content {
		t.Errorf("file content = %q, want %q", string(diskBytes), content)
	}
}

func TestArtifactGet_CalledTwice_SamePath(t *testing.T) {
	env := newBuiltinTestEnv(t)

	if _, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"dup.txt","content":"same"}`); err != nil || isErr {
		t.Fatalf("add artifact: isErr=%v err=%v", isErr, err)
	}

	out1, _, _ := invoke(t, env.env, "artifact_get", `{"name":"dup.txt"}`)
	out2, _, _ := invoke(t, env.env, "artifact_get", `{"name":"dup.txt"}`)

	var r1, r2 map[string]string
	json.Unmarshal([]byte(out1), &r1)
	json.Unmarshal([]byte(out2), &r2)

	if r1["path"] != r2["path"] {
		t.Errorf("first path=%q, second path=%q, want same", r1["path"], r2["path"])
	}

	fi1, err := os.Stat(r1["path"])
	if err != nil {
		t.Fatalf("Stat first: %v", err)
	}
	fi2, err := os.Stat(r2["path"])
	if err != nil {
		t.Fatalf("Stat second: %v", err)
	}
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Errorf("mtime changed between calls: first=%v second=%v", fi1.ModTime(), fi2.ModTime())
	}
}

func TestArtifactGet_UnknownName_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_get", `{"name":"nonexistent.txt"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for unknown artifact")
	}
	if !strings.Contains(out, "nonexistent.txt") {
		t.Errorf("output=%q, want artifact name in error", out)
	}
}

func TestArtifactAdd_MissingName_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `{"content":"data"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for missing name")
	}
	if !strings.Contains(out, "name") {
		t.Errorf("output=%q, want 'name' mention", out)
	}
}

func TestArtifactAdd_MissingContent_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `{"name":"f.txt"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for missing content")
	}
	_ = out
}

func TestArtifactGet_MissingName_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_get", `{}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for missing name")
	}
	_ = out
}

func TestArtifactAdd_InvalidJSON_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "artifact_add", `not-json`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for invalid JSON")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output=%q, want 'invalid arguments'", out)
	}
}
