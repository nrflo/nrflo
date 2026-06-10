package cli

import (
	"encoding/json"
	"errors"
	"testing"
)

// fakeMCPCaller is a test double for mcpSocketCaller.
type fakeMCPCaller struct {
	toolsListResult json.RawMessage
	toolsListErr    error
	toolsCallResult json.RawMessage
	toolsCallErr    error
}

func (f *fakeMCPCaller) Call(method string, params map[string]interface{}) (json.RawMessage, error) {
	switch method {
	case "tools.list":
		return f.toolsListResult, f.toolsListErr
	case "tools.call":
		return f.toolsCallResult, f.toolsCallErr
	default:
		return nil, errors.New("unexpected method: " + method)
	}
}

func makeMCPReq(id interface{}, method string, paramsJSON string) mcpRequest {
	req := mcpRequest{JSONRPC: "2.0", ID: id, Method: method}
	if paramsJSON != "" {
		req.Params = json.RawMessage(paramsJSON)
	}
	return req
}

// TestDispatchMCP_Initialize verifies capabilities and serverInfo shape.
func TestDispatchMCP_Initialize(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(1, "initialize", ""), "ses1", "ins1", false, caller)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not map: %T", resp.Result)
	}

	caps, _ := result["capabilities"].(map[string]interface{})
	if caps == nil {
		t.Fatal("capabilities missing")
	}
	if _, ok := caps["tools"]; !ok {
		t.Error("capabilities.tools missing")
	}

	info, _ := result["serverInfo"].(map[string]interface{})
	if info == nil {
		t.Fatal("serverInfo missing")
	}
	if info["name"] != "nrflo" {
		t.Errorf("serverInfo.name = %v, want nrflo", info["name"])
	}
}

// TestDispatchMCP_Initialize_IDPreserved verifies ID is echoed back.
func TestDispatchMCP_Initialize_IDPreserved(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq("req-abc", "initialize", ""), "s", "i", false, caller)
	if resp.ID != "req-abc" {
		t.Errorf("ID = %v, want req-abc", resp.ID)
	}
}

// TestDispatchMCP_NotificationsInitialized returns nil (no reply for notifications).
func TestDispatchMCP_NotificationsInitialized(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(nil, "notifications/initialized", ""), "s", "i", false, caller)
	if resp != nil {
		t.Errorf("expected nil response for notification, got %+v", resp)
	}
}

// TestDispatchMCP_ToolsList_HappyPath returns bare tool names array.
func TestDispatchMCP_ToolsList_HappyPath(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsListResult: json.RawMessage(`[{"name":"tool_a"},{"name":"tool_b"}]`),
	}
	resp := dispatchMCP(makeMCPReq(2, "tools/list", ""), "ses1", "ins1", false, caller)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}
	tools, _ := result["tools"].([]json.RawMessage)
	if len(tools) != 2 {
		t.Errorf("tools len = %d, want 2", len(tools))
	}
}

// TestDispatchMCP_ToolsList_EmptyArray returns empty tools array (not null).
func TestDispatchMCP_ToolsList_EmptyArray(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsListResult: json.RawMessage(`[]`),
	}
	resp := dispatchMCP(makeMCPReq(3, "tools/list", ""), "s", "i", false, caller)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]json.RawMessage)
	if tools == nil {
		t.Error("tools should not be nil")
	}
	if len(tools) != 0 {
		t.Errorf("tools len = %d, want 0", len(tools))
	}
}

// TestDispatchMCP_ToolsList_CallerError returns -32603.
func TestDispatchMCP_ToolsList_CallerError(t *testing.T) {
	caller := &fakeMCPCaller{toolsListErr: errors.New("socket unavailable")}
	resp := dispatchMCP(makeMCPReq(4, "tools/list", ""), "s", "i", false, caller)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("code = %d, want -32603", resp.Error.Code)
	}
}

// TestDispatchMCP_ToolsCall_HappyPath returns {content:[{type:text,text}],isError}.
func TestDispatchMCP_ToolsCall_HappyPath(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsCallResult: json.RawMessage(`{"output":"hello world","is_error":false}`),
	}
	resp := dispatchMCP(makeMCPReq(5, "tools/call", `{"name":"my_tool","arguments":{"x":1}}`), "s", "i", false, caller)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}
	content, _ := result["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("content[0].type = %v, want text", content[0]["type"])
	}
	if content[0]["text"] != "hello world" {
		t.Errorf("content[0].text = %v, want 'hello world'", content[0]["text"])
	}
	if result["isError"] != false {
		t.Errorf("isError = %v, want false", result["isError"])
	}
}

// TestDispatchMCP_ToolsCall_IsError verifies isError=true forwarded.
func TestDispatchMCP_ToolsCall_IsError(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsCallResult: json.RawMessage(`{"output":"failed","is_error":true}`),
	}
	resp := dispatchMCP(makeMCPReq(6, "tools/call", `{"name":"bad_tool"}`), "s", "i", false, caller)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	if result["isError"] != true {
		t.Errorf("isError = %v, want true", result["isError"])
	}
}

// TestDispatchMCP_ToolsCall_MissingName returns -32602.
func TestDispatchMCP_ToolsCall_MissingName(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(7, "tools/call", `{"name":""}`), "s", "i", false, caller)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code = %d, want -32602", resp.Error.Code)
	}
}

// TestDispatchMCP_ToolsCall_NilParams uses {} as input.
func TestDispatchMCP_ToolsCall_NilParams(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsCallResult: json.RawMessage(`{"output":"ok","is_error":false}`),
	}
	// Params is nil — should treat name as empty → -32602
	req := mcpRequest{JSONRPC: "2.0", ID: 8, Method: "tools/call", Params: nil}
	resp := dispatchMCP(req, "s", "i", false, caller)
	if resp.Error == nil {
		t.Fatal("expected error when name is empty (nil params)")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code = %d, want -32602", resp.Error.Code)
	}
}

// TestDispatchMCP_ToolsCall_NilArguments defaults arguments to {}.
func TestDispatchMCP_ToolsCall_NilArguments(t *testing.T) {
	caller := &fakeMCPCaller{
		toolsCallResult: json.RawMessage(`{"output":"ok","is_error":false}`),
	}
	resp := dispatchMCP(makeMCPReq(9, "tools/call", `{"name":"tool_x"}`), "s", "i", false, caller)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

// TestDispatchMCP_ToolsCall_InvalidParams returns -32602.
func TestDispatchMCP_ToolsCall_InvalidParams(t *testing.T) {
	caller := &fakeMCPCaller{}
	resp := dispatchMCP(makeMCPReq(10, "tools/call", `{not json`), "s", "i", false, caller)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code = %d, want -32602", resp.Error.Code)
	}
}

// TestDispatchMCP_ToolsCall_CallerError returns -32603.
func TestDispatchMCP_ToolsCall_CallerError(t *testing.T) {
	caller := &fakeMCPCaller{toolsCallErr: errors.New("socket unavailable")}
	resp := dispatchMCP(makeMCPReq(11, "tools/call", `{"name":"tool_x"}`), "s", "i", false, caller)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("code = %d, want -32603", resp.Error.Code)
	}
}
