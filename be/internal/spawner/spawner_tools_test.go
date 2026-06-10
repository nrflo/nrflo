package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// plainToolHandler returns a fixed (output, isError) pair.
type plainToolHandler struct {
	spec    provider.ToolSpec
	output  string
	isError bool
}

func (h *plainToolHandler) Spec() provider.ToolSpec { return h.spec }
func (h *plainToolHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return h.output, h.isError, nil
}

// terminalToolHandler returns a TerminalSignal error.
type terminalToolHandler struct {
	spec   provider.ToolSpec
	status string
}

func (h *terminalToolHandler) Spec() provider.ToolSpec { return h.spec }
func (h *terminalToolHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return "", false, apirun.TerminalSignal{Status: h.status}
}

// errorToolHandler returns a plain error.
type errorToolHandler struct {
	spec provider.ToolSpec
	err  error
}

func (h *errorToolHandler) Spec() provider.ToolSpec { return h.spec }
func (h *errorToolHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return "", false, h.err
}

// mediaToolHandler implements MediaToolHandler.
type mediaToolHandler struct {
	spec   provider.ToolSpec
	output string
}

func (h *mediaToolHandler) Spec() provider.ToolSpec { return h.spec }
func (h *mediaToolHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return h.output, false, nil
}
func (h *mediaToolHandler) InvokeMedia(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, []provider.MediaBlock, bool, error) {
	return h.output, nil, false, nil
}

func makeSpec(name, desc string) provider.ToolSpec {
	return provider.ToolSpec{
		Name:        name,
		Description: desc,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func newTestSpawner() *Spawner {
	return New(Config{Clock: clock.Real()})
}

func registerProc(s *Spawner, sessionID string, tools []provider.ToolSpec, handlers apirun.Registry) {
	proc := &processInfo{
		sessionID:   sessionID,
		apiViaCLI:   true,
		apiTools:    tools,
		apiHandlers: handlers,
	}
	s.registerSessionProc(sessionID, proc)
}

// TestListTools_UnknownSession returns empty array without error.
func TestListTools_UnknownSession(t *testing.T) {
	s := newTestSpawner()
	got, err := s.ListTools("no-such-session")
	if err != nil {
		t.Fatalf("ListTools unknown session: %v", err)
	}
	if string(got) != "[]" {
		t.Errorf("ListTools unknown session = %s, want []", got)
	}
}

// TestListTools_Empty returns "[]" for a proc with no tools.
func TestListTools_Empty(t *testing.T) {
	s := newTestSpawner()
	registerProc(s, "sess-empty", nil, apirun.Registry{})
	got, err := s.ListTools("sess-empty")
	if err != nil {
		t.Fatalf("ListTools empty: %v", err)
	}
	if string(got) != "[]" {
		t.Errorf("ListTools empty = %s, want []", got)
	}
}

// TestListTools_Shape verifies name/description/inputSchema fields are present.
func TestListTools_Shape(t *testing.T) {
	s := newTestSpawner()
	spec := makeSpec("my_tool", "does things")
	registerProc(s, "sess-1", []provider.ToolSpec{spec}, apirun.Registry{
		"my_tool": &plainToolHandler{spec: spec},
	})
	raw, err := s.ListTools("sess-1")
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var entries []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].Name != "my_tool" {
		t.Errorf("name = %q, want my_tool", entries[0].Name)
	}
	if entries[0].Description != "does things" {
		t.Errorf("description = %q, want 'does things'", entries[0].Description)
	}
	if string(entries[0].InputSchema) != `{"type":"object"}` {
		t.Errorf("inputSchema = %s, want {\"type\":\"object\"}", entries[0].InputSchema)
	}
}

// TestListTools_MultipleTools verifies all tools are returned.
func TestListTools_MultipleTools(t *testing.T) {
	s := newTestSpawner()
	specs := []provider.ToolSpec{makeSpec("tool_a", "a"), makeSpec("tool_b", "b")}
	handlers := apirun.Registry{
		"tool_a": &plainToolHandler{spec: specs[0]},
		"tool_b": &plainToolHandler{spec: specs[1]},
	}
	registerProc(s, "sess-multi", specs, handlers)
	raw, err := s.ListTools("sess-multi")
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len = %d, want 2", len(entries))
	}
}
