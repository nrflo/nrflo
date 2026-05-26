package socket

import (
	"encoding/json"
	"fmt"
	"testing"
)

// fakeToolDispatcher records ListTools/CallTool calls for assertion.
type fakeToolDispatcher struct {
	listErr    error
	listResult json.RawMessage
	callOutput string
	callIsErr  bool
	callErr    error

	listCalls []toolListCall
	callCalls []toolCallCall
}

type toolListCall struct {
	instanceID string
	sessionID  string
}

type toolCallCall struct {
	instanceID string
	sessionID  string
	name       string
}

func (f *fakeToolDispatcher) ListTools(instanceID, sessionID string) (json.RawMessage, error) {
	f.listCalls = append(f.listCalls, toolListCall{instanceID: instanceID, sessionID: sessionID})
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listResult != nil {
		return f.listResult, nil
	}
	return json.RawMessage(`[]`), nil
}

func (f *fakeToolDispatcher) CallTool(instanceID, sessionID, name string, input json.RawMessage) (string, bool, error) {
	f.callCalls = append(f.callCalls, toolCallCall{
		instanceID: instanceID, sessionID: sessionID, name: name,
	})
	return f.callOutput, f.callIsErr, f.callErr
}

func buildToolsReq(id, action string, params string) Request {
	return Request{
		ID:     id,
		Method: "tools." + action,
		Params: json.RawMessage(params),
	}
}

// TestHandleTools_NilDispatcher returns internal error for both list and call.
func TestHandleTools_NilDispatcher(t *testing.T) {
	h := &Handler{toolDispatcher: nil}
	for _, action := range []string{"list", "call"} {
		t.Run(action, func(t *testing.T) {
			req := buildToolsReq("r1", action, `{"session_id":"s","instance_id":"i"}`)
			resp := h.handleTools(req, action)
			if resp.Error == nil {
				t.Fatal("expected error, got nil")
			}
			if resp.Error.Code != ErrCodeInternal {
				t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeInternal)
			}
		})
	}
}

// TestHandleTools_List_HappyPath verifies tools.list returns array and threads IDs.
func TestHandleTools_List_HappyPath(t *testing.T) {
	td := &fakeToolDispatcher{
		listResult: json.RawMessage(`[{"name":"tool_a"}]`),
	}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r2", "list", `{"session_id":"ses1","instance_id":"ins1"}`)
	resp := h.handleTools(req, "list")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result) != 1 || result[0]["name"] != "tool_a" {
		t.Errorf("result = %v, want [{name:tool_a}]", result)
	}

	if len(td.listCalls) != 1 {
		t.Fatalf("listCalls = %d, want 1", len(td.listCalls))
	}
	if td.listCalls[0].instanceID != "ins1" {
		t.Errorf("instanceID = %q, want ins1", td.listCalls[0].instanceID)
	}
	if td.listCalls[0].sessionID != "ses1" {
		t.Errorf("sessionID = %q, want ses1", td.listCalls[0].sessionID)
	}
}

// TestHandleTools_List_ServiceError returns internal error.
func TestHandleTools_List_ServiceError(t *testing.T) {
	td := &fakeToolDispatcher{listErr: fmt.Errorf("dispatcher failure")}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r3", "list", `{"session_id":"s","instance_id":"i"}`)
	resp := h.handleTools(req, "list")
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeInternal)
	}
}

// TestHandleTools_List_InvalidParams returns invalid-params error.
func TestHandleTools_List_InvalidParams(t *testing.T) {
	td := &fakeToolDispatcher{}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r4", "list", `{not valid json`)
	resp := h.handleTools(req, "list")
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeInvalidParams)
	}
}

// TestHandleTools_Call_HappyPath verifies tools.call returns {output, is_error}.
func TestHandleTools_Call_HappyPath(t *testing.T) {
	td := &fakeToolDispatcher{callOutput: "result text", callIsErr: false}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r5", "call", `{"session_id":"s2","instance_id":"i2","name":"my_tool","input":{"x":1}}`)
	resp := h.handleTools(req, "call")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["output"] != "result text" {
		t.Errorf("output = %v, want 'result text'", result["output"])
	}
	if result["is_error"] != false {
		t.Errorf("is_error = %v, want false", result["is_error"])
	}

	if len(td.callCalls) != 1 {
		t.Fatalf("callCalls = %d, want 1", len(td.callCalls))
	}
	c := td.callCalls[0]
	if c.name != "my_tool" {
		t.Errorf("name = %q, want my_tool", c.name)
	}
	if c.sessionID != "s2" {
		t.Errorf("sessionID = %q, want s2", c.sessionID)
	}
	if c.instanceID != "i2" {
		t.Errorf("instanceID = %q, want i2", c.instanceID)
	}
}

// TestHandleTools_Call_IsError verifies is_error=true propagated.
func TestHandleTools_Call_IsError(t *testing.T) {
	td := &fakeToolDispatcher{callOutput: "bad output", callIsErr: true}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r6", "call", `{"session_id":"s","instance_id":"i","name":"err_tool"}`)
	resp := h.handleTools(req, "call")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Result, &result)
	if result["is_error"] != true {
		t.Errorf("is_error = %v, want true", result["is_error"])
	}
	if result["output"] != "bad output" {
		t.Errorf("output = %v, want 'bad output'", result["output"])
	}
}

// TestHandleTools_Call_MissingName returns validation error.
func TestHandleTools_Call_MissingName(t *testing.T) {
	td := &fakeToolDispatcher{}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r7", "call", `{"session_id":"s","instance_id":"i","name":""}`)
	resp := h.handleTools(req, "call")
	if resp.Error == nil {
		t.Fatal("expected error for missing name")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestHandleTools_Call_ServiceError returns internal error.
func TestHandleTools_Call_ServiceError(t *testing.T) {
	td := &fakeToolDispatcher{callErr: fmt.Errorf("call failure")}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r8", "call", `{"session_id":"s","instance_id":"i","name":"tool"}`)
	resp := h.handleTools(req, "call")
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeInternal)
	}
}

// TestHandleTools_Call_InvalidParams returns invalid-params error.
func TestHandleTools_Call_InvalidParams(t *testing.T) {
	td := &fakeToolDispatcher{}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r9", "call", `{bad json`)
	resp := h.handleTools(req, "call")
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeInvalidParams)
	}
}

// TestHandleTools_UnknownAction returns method-not-found error.
func TestHandleTools_UnknownAction(t *testing.T) {
	td := &fakeToolDispatcher{}
	h := &Handler{toolDispatcher: td}

	req := buildToolsReq("r10", "delete", `{}`)
	resp := h.handleTools(req, "delete")
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

// TestHandleTools_RoutedViaHandle verifies tools.list/call are dispatched by Handle.
func TestHandleTools_RoutedViaHandle(t *testing.T) {
	env := newHandlerTestEnv(t)

	td := &fakeToolDispatcher{}
	env.handler.toolDispatcher = td

	req := Request{
		ID:      "route-1",
		Method:  "tools.list",
		Project: env.project,
		Params:  json.RawMessage(`{"session_id":"s","instance_id":"i"}`),
	}
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("Handle tools.list: %v", resp.Error)
	}
	if len(td.listCalls) != 1 {
		t.Errorf("listCalls = %d, want 1", len(td.listCalls))
	}
}
