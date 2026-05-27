package cli

import (
	"encoding/json"
	"errors"
	"testing"
)

// recordingCaller records the last method/params and returns a canned result.
type recordingCaller struct {
	method string
	params map[string]interface{}
	result json.RawMessage
	err    error
}

func (r *recordingCaller) Call(method string, params map[string]interface{}) (json.RawMessage, error) {
	r.method, r.params = method, params
	return r.result, r.err
}

// TestDispatchMCP_Observer_ToolsList returns the static observer tool set.
func TestDispatchMCP_Observer_ToolsList(t *testing.T) {
	resp := dispatchMCP(makeMCPReq(1, "tools/list", ""), "obs-1", "", true, &recordingCaller{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	tools := resp.Result.(map[string]interface{})["tools"].([]json.RawMessage)
	if len(tools) != len(observerMethodList) {
		t.Fatalf("tools len = %d, want %d", len(tools), len(observerMethodList))
	}
	// Spot-check a couple of expected tool names.
	names := map[string]bool{}
	for _, raw := range tools {
		var spec struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &spec)
		names[spec.Name] = true
	}
	for _, want := range []string{"observer_workflow_show", "observer_project_env_set", "observer_global_projects"} {
		if !names[want] {
			t.Errorf("observer tools missing %q", want)
		}
	}
}

// TestDispatchMCP_Observer_ToolsCall maps the tool to its observer.* socket
// method, injects session_id, flattens input, and returns the raw result.
func TestDispatchMCP_Observer_ToolsCall(t *testing.T) {
	caller := &recordingCaller{result: json.RawMessage(`{"phases":[]}`)}
	resp := dispatchMCP(
		makeMCPReq(2, "tools/call", `{"name":"observer_workflow_show","arguments":{"workflow_id":"wf-1"}}`),
		"obs-1", "", true, caller)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if caller.method != "observer.workflow.show" {
		t.Errorf("socket method = %q, want observer.workflow.show", caller.method)
	}
	if caller.params["session_id"] != "obs-1" {
		t.Errorf("session_id = %v, want obs-1", caller.params["session_id"])
	}
	if caller.params["workflow_id"] != "wf-1" {
		t.Errorf("workflow_id = %v, want wf-1 (input not flattened)", caller.params["workflow_id"])
	}
	result := resp.Result.(map[string]interface{})
	if result["isError"] != false {
		t.Errorf("isError = %v, want false", result["isError"])
	}
	content := result["content"].([]map[string]interface{})
	if content[0]["text"] != `{"phases":[]}` {
		t.Errorf("text = %v, want raw socket result", content[0]["text"])
	}
}

// TestDispatchMCP_Observer_SocketError surfaces socket/authorization errors as an
// error result (isError=true) rather than a JSON-RPC error, so the model sees it.
func TestDispatchMCP_Observer_SocketError(t *testing.T) {
	caller := &recordingCaller{err: errors.New("permission denied: out-of-scope call")}
	resp := dispatchMCP(
		makeMCPReq(3, "tools/call", `{"name":"observer_global_projects"}`),
		"obs-1", "", true, caller)
	if resp.Error != nil {
		t.Fatalf("expected tool-result error, got JSON-RPC error: %v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	if result["isError"] != true {
		t.Errorf("isError = %v, want true", result["isError"])
	}
	content := result["content"].([]map[string]interface{})
	if content[0]["text"] != "permission denied: out-of-scope call" {
		t.Errorf("text = %v, want the socket error message", content[0]["text"])
	}
}

// TestDispatchMCP_Observer_UnknownTool returns -32602 for an unmapped tool name.
func TestDispatchMCP_Observer_UnknownTool(t *testing.T) {
	resp := dispatchMCP(
		makeMCPReq(4, "tools/call", `{"name":"observer_bogus"}`),
		"obs-1", "", true, &recordingCaller{})
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("error = %v, want code -32602", resp.Error)
	}
}

// TestObserverToolNameMapping verifies the dotted<->underscore mapping round-trips.
func TestObserverToolNameMapping(t *testing.T) {
	method, ok := observerSocketMethod("observer_workflow_def_update")
	if !ok || method != "observer.workflow.def.update" {
		t.Errorf("observerSocketMethod = %q,%v, want observer.workflow.def.update,true", method, ok)
	}
	if observerToolName("project.env.set") != "observer_project_env_set" {
		t.Errorf("observerToolName mismatch: %q", observerToolName("project.env.set"))
	}
}
