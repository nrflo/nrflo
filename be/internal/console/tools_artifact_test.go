package console

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/model"
)

// addArtifact writes a real artifact blob via ArtifactService.AddFromAgent
// (internal-fs storage rooted at NRFLO_HOME, set by newConsoleTestEnv) so
// artifact_list/artifact_get exercise the real storage + repo path.
func (e *consoleTestEnv) addArtifact(t *testing.T, projectID, wfiID, name, contentType string, data []byte) {
	t.Helper()
	if _, err := e.deps.ArtifactSvc.AddFromAgent(context.Background(), "sess-seed", projectID, wfiID, name, contentType, data); err != nil {
		t.Fatalf("AddFromAgent(%s): %v", name, err)
	}
}

func TestArtifactList_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-art-other")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_list", `{"instance_id":"wfi-art-other"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestArtifactList_HappyPath(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-art-own")
	env.addArtifact(t, testProjectID, "wfi-art-own", "notes.txt", "text/plain", []byte("hello world"))
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_list", `{"instance_id":"wfi-art-own"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var items []map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &items); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	if len(items) != 1 || items[0]["name"] != "notes.txt" {
		t.Errorf("items = %v, want one entry named notes.txt", items)
	}
}

func TestArtifactGet_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-artget-other")
	env.addArtifact(t, testOtherProjectID, "wfi-artget-other", "secret.txt", "text/plain", []byte("nope"))
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_get", `{"instance_id":"wfi-artget-other","name":"secret.txt"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestArtifactGet_TextContent_InlinedAsText(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-artget-own")
	env.addArtifact(t, testProjectID, "wfi-artget-own", "notes.txt", "text/plain", []byte("hello world"))
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_get", `{"instance_id":"wfi-artget-own","name":"notes.txt"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var resp map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &resp); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	if resp["encoding"] != "text" || resp["content"] != "hello world" {
		t.Errorf("resp = %v, want text encoding with content 'hello world'", resp)
	}
}

func TestArtifactGet_BinaryContent_InlinedAsBase64(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-artget-bin")
	binary := []byte{0x00, 0x01, 0xff, 0xfe, 0x80}
	env.addArtifact(t, testProjectID, "wfi-artget-bin", "blob.bin", "application/octet-stream", binary)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_get", `{"instance_id":"wfi-artget-bin","name":"blob.bin"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var resp map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &resp); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	if resp["encoding"] != "base64" {
		t.Errorf("encoding = %v, want base64", resp["encoding"])
	}
}

func TestArtifactGet_NotFound_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-artget-missing")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "artifact_get", `{"instance_id":"wfi-artget-missing","name":"nope.txt"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "artifact not found") {
		t.Errorf("out=%q isErr=%v, want artifact not found", out, isErr)
	}
}
