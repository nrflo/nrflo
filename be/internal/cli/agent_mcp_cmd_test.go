package cli

// Command structure and edge-case dispatch tests — split from agent_mcp_test.go
// to stay under 300 lines.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDispatchMCP_UnknownMethod returns -32601.
func TestDispatchMCP_UnknownMethod(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(12, "custom/method", ""), "s", "i", caller)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code = %d, want -32601", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "method not found") {
		t.Errorf("message = %q, want 'method not found'", resp.Error.Message)
	}
}

// TestDispatchMCP_JSONRPCVersion verifies responses always have "2.0".
func TestDispatchMCP_JSONRPCVersion(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(1, "initialize", ""), "s", "i", caller)
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
}

// TestAgentMCPCmd_RegisteredUnderAgent verifies 'mcp' is a subcommand of agentCmd.
func TestAgentMCPCmd_RegisteredUnderAgent(t *testing.T) {
	subcmds := getCommandNames(agentCmd)
	found := false
	for _, s := range subcmds {
		if s == "mcp" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("agentCmd missing 'mcp' subcommand. Got: %v", subcmds)
	}
}

// TestAgentMCPCmd_Structure verifies command metadata.
func TestAgentMCPCmd_Structure(t *testing.T) {
	if agentMCPCmd.Use != "mcp" {
		t.Errorf("Use = %q, want mcp", agentMCPCmd.Use)
	}
	if agentMCPCmd.Short == "" {
		t.Error("Short should not be empty")
	}
	if agentMCPCmd.RunE == nil {
		t.Error("RunE should not be nil")
	}
	if err := agentMCPCmd.Args(agentMCPCmd, []string{}); err != nil {
		t.Errorf("mcp should accept 0 args: %v", err)
	}
	if err := agentMCPCmd.Args(agentMCPCmd, []string{"unexpected"}); err == nil {
		t.Error("mcp should reject positional args")
	}
}

// TestDispatchMCP_ToolsList_BadJSON returns -32603 when response is unparseable.
func TestDispatchMCP_ToolsList_BadJSON(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsListResult: json.RawMessage(`{not an array}`),
	}
	resp := dispatchMCP(makeMCPReq(13, "tools/list", ""), "s", "i", caller)
	if resp.Error == nil {
		t.Fatal("expected error for bad JSON array")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("code = %d, want -32603", resp.Error.Code)
	}
}
