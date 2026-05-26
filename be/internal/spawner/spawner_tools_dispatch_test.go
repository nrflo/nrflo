package spawner

// DispatchTool tests — split from spawner_tools_test.go to stay under 300 lines.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

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
