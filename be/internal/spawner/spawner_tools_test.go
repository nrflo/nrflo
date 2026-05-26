package spawner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// TestDispatchTool_UnknownSession returns isError=true without error.
func TestDispatchTool_UnknownSession(t *testing.T) {
	s := newTestSpawner()
	out, isErr, terminal, err := s.DispatchTool("no-session", "my_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErr {
		t.Error("isError should be true for unknown session")
	}
	if !strings.Contains(out, "no-session") {
		t.Errorf("output %q should mention session ID", out)
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}

// TestDispatchTool_UnknownTool returns isError=true without error.
func TestDispatchTool_UnknownTool(t *testing.T) {
	s := newTestSpawner()
	registerProc(s, "sess-2", nil, apirun.Registry{})
	out, isErr, terminal, err := s.DispatchTool("sess-2", "no_such_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErr {
		t.Error("isError should be true for unknown tool")
	}
	if !strings.Contains(out, "no_such_tool") {
		t.Errorf("output %q should mention tool name", out)
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}

// TestDispatchTool_PlainInvoke returns output and isError from handler.
func TestDispatchTool_PlainInvoke(t *testing.T) {
	s := newTestSpawner()
	spec := makeSpec("echo", "echo tool")
	h := &plainToolHandler{spec: spec, output: "hello", isError: false}
	registerProc(s, "sess-3", []provider.ToolSpec{spec}, apirun.Registry{"echo": h})

	out, isErr, terminal, err := s.DispatchTool("sess-3", "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
	if isErr {
		t.Error("isError should be false")
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}

// TestDispatchTool_IsErrorTrue verifies isError=true propagated from handler.
func TestDispatchTool_IsErrorTrue(t *testing.T) {
	s := newTestSpawner()
	spec := makeSpec("bad", "bad tool")
	h := &plainToolHandler{spec: spec, output: "oops", isError: true}
	registerProc(s, "sess-4", []provider.ToolSpec{spec}, apirun.Registry{"bad": h})

	out, isErr, terminal, err := s.DispatchTool("sess-4", "bad", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "oops" {
		t.Errorf("output = %q, want oops", out)
	}
	if !isErr {
		t.Error("isError should be true")
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}

// TestDispatchTool_TerminalSignal returns lowercased status and no error.
func TestDispatchTool_TerminalSignal(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"PASS", "pass"},
		{"FAIL", "fail"},
		{"CONTINUE", "continue"},
		{"CALLBACK", "callback"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			s := newTestSpawner()
			spec := makeSpec("terminal_tool", "terminal")
			h := &terminalToolHandler{spec: spec, status: tc.status}
			registerProc(s, "sess-ts-"+tc.status, []provider.ToolSpec{spec}, apirun.Registry{"terminal_tool": h})

			out, isErr, terminal, err := s.DispatchTool("sess-ts-"+tc.status, "terminal_tool", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != "" {
				t.Errorf("output = %q, want empty", out)
			}
			if isErr {
				t.Error("isError should be false on TerminalSignal")
			}
			if terminal != tc.want {
				t.Errorf("terminal = %q, want %q", terminal, tc.want)
			}
		})
	}
}

// TestDispatchTool_PlainError returns (msg, isError=true, "", nil).
func TestDispatchTool_PlainError(t *testing.T) {
	s := newTestSpawner()
	spec := makeSpec("err_tool", "error tool")
	h := &errorToolHandler{spec: spec, err: errors.New("something went wrong")}
	registerProc(s, "sess-5", []provider.ToolSpec{spec}, apirun.Registry{"err_tool": h})

	out, isErr, terminal, err := s.DispatchTool("sess-5", "err_tool", nil)
	if err != nil {
		t.Fatalf("unexpected outer error: %v", err)
	}
	if !isErr {
		t.Error("isError should be true on plain error")
	}
	if !strings.Contains(out, "something went wrong") {
		t.Errorf("output %q should contain error message", out)
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}

// TestDispatchTool_MediaHandler prefers InvokeMedia path.
func TestDispatchTool_MediaHandler(t *testing.T) {
	s := newTestSpawner()
	spec := makeSpec("read_doc", "media tool")
	h := &mediaToolHandler{spec: spec, output: "doc text"}
	registerProc(s, "sess-6", []provider.ToolSpec{spec}, apirun.Registry{"read_doc": h})

	out, isErr, terminal, err := s.DispatchTool("sess-6", "read_doc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "doc text" {
		t.Errorf("output = %q, want 'doc text'", out)
	}
	if isErr {
		t.Error("isError should be false")
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}
}
