package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDispatchExternalMCP_Initialize(t *testing.T) {
	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "initialize", ""), &nrfloHTTPClient{})
	res, ok := resp.Result.(map[string]interface{})
	if !ok || res["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected initialize result: %+v", resp)
	}
}

func TestDispatchExternalMCP_NotificationsInitialized(t *testing.T) {
	if resp := dispatchExternalMCP(context.Background(), makeMCPReq(nil, "notifications/initialized", ""), &nrfloHTTPClient{}); resp != nil {
		t.Fatalf("expected nil response for notification, got %+v", resp)
	}
}

func TestDispatchExternalMCP_UnknownMethod(t *testing.T) {
	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "nope", ""), &nrfloHTTPClient{})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected method-not-found error, got %+v", resp)
	}
}

// TestDispatchExternalMCP_ToolsList_PassthroughAndSchemaDefault covers case 3:
// tools/list is a pure passthrough of the server-owned catalogue, and an entry
// with no schema defaults to {"type":"object"}.
func TestDispatchExternalMCP_ToolsList_PassthroughAndSchemaDefault(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.tools = []consoleTool{
		{Name: "project_status", Description: "status", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "artifact_list", Description: "list artifacts"}, // no schema -> defaults
	}
	c := f.openedClient(t, "p1")

	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "tools/list", ""), c)
	res, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result shape: %+v", resp)
	}
	tools, ok := res["tools"].([]map[string]interface{})
	if !ok || len(tools) != 2 {
		t.Fatalf("got tools=%+v, want 2 entries", res["tools"])
	}
	byName := map[string]map[string]interface{}{}
	for _, tl := range tools {
		byName[tl["name"].(string)] = tl
	}
	if got := string(byName["project_status"]["inputSchema"].(json.RawMessage)); got != `{"type":"object","properties":{}}` {
		t.Errorf("project_status inputSchema = %s, want passthrough", got)
	}
	if got := string(byName["artifact_list"]["inputSchema"].(json.RawMessage)); got != `{"type":"object"}` {
		t.Errorf(`artifact_list inputSchema default = %s, want {"type":"object"}`, got)
	}
	if len(f.listReqs) != 1 || f.listReqs[0].auth != "Bearer "+f.sessionToken {
		t.Errorf("tools/list should use the console token, got %+v (session token %q)", f.listReqs, f.sessionToken)
	}
}

// TestDispatchExternalMCP_ToolsCall_PassthroughAndArgsDefault covers case 4:
// request path, raw-args passthrough, and absent arguments defaulting to {}.
func TestDispatchExternalMCP_ToolsCall_PassthroughAndArgsDefault(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.toolResp["project_status"] = toolCallResp{output: "X", isError: false}
	c := f.openedClient(t, "p1")

	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "tools/call", `{"name":"project_status","arguments":{"a":1}}`), c)
	res := resp.Result.(map[string]interface{})
	content := res["content"].([]map[string]interface{})
	if content[0]["text"] != "X" {
		t.Errorf("content text = %v, want X", content[0]["text"])
	}
	if res["isError"] != false {
		t.Errorf("isError = %v, want false", res["isError"])
	}
	if len(f.callReqs) != 1 {
		t.Fatalf("call count = %d, want 1", len(f.callReqs))
	}
	got := f.callReqs[0]
	if got.name != "project_status" || string(got.args) != `{"a":1}` {
		t.Errorf("call req = %+v, want raw args passthrough", got)
	}
	if got.auth != "Bearer "+f.sessionToken {
		t.Errorf("tools/call should use the console token, got %q", got.auth)
	}

	// Second call with no arguments at all: bridge must send {}.
	resp2 := dispatchExternalMCP(context.Background(), makeMCPReq(2, "tools/call", `{"name":"project_status"}`), c)
	if res2 := resp2.Result.(map[string]interface{}); res2["isError"] != false {
		t.Errorf("second call isError = %v, want false", res2["isError"])
	}
	if len(f.callReqs) != 2 || string(f.callReqs[1].args) != "{}" {
		t.Errorf("absent arguments should be sent as {}, got %q", f.callReqs[len(f.callReqs)-1].args)
	}
}

func TestDispatchExternalMCP_ToolsCall_IsErrorTrue(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.toolResp["artifact_list"] = toolCallResp{output: "boom", isError: true}
	c := f.openedClient(t, "p1")

	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "tools/call", `{"name":"artifact_list"}`), c)
	res := resp.Result.(map[string]interface{})
	if res["isError"] != true {
		t.Errorf("isError = %v, want true", res["isError"])
	}
	content := res["content"].([]map[string]interface{})
	if content[0]["text"] != "boom" {
		t.Errorf("content text = %v, want boom", content[0]["text"])
	}
}

// TestDispatchExternalMCP_ToolsCall_HTTPErrorsBecomeIsError covers case 5:
// both an unlisted-tool 404 and a server 500 come back as isError:true tool
// results carrying the server's message, not JSON-RPC errors.
func TestDispatchExternalMCP_ToolsCall_HTTPErrorsBecomeIsError(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(f *fakeConsoleServer)
		toolName   string
		wantSubstr string
	}{
		{name: "404 unlisted tool", setup: func(f *fakeConsoleServer) {}, toolName: "unknown_tool", wantSubstr: "unknown tool"},
		{
			name:       "500 server error",
			setup:      func(f *fakeConsoleServer) { f.toolResp["project_status"] = toolCallResp{status: 500, errBody: "boom"} },
			toolName:   "project_status",
			wantSubstr: "boom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeConsoleServer(t)
			tc.setup(f)
			c := f.openedClient(t, "p1")
			resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "tools/call", `{"name":"`+tc.toolName+`"}`), c)
			if resp.Error != nil {
				t.Fatalf("expected a tool-result error, not a JSON-RPC error: %+v", resp.Error)
			}
			res := resp.Result.(map[string]interface{})
			if res["isError"] != true {
				t.Fatalf("isError = %v, want true", res["isError"])
			}
			content := res["content"].([]map[string]interface{})
			text, _ := content[0]["text"].(string)
			if !strings.Contains(text, tc.wantSubstr) {
				t.Errorf("error text = %q, want substring %q", text, tc.wantSubstr)
			}
		})
	}
}

// TestRunE_MissingToken covers case 8: no NRFLO_MCP_TOKEN errors out of RunE
// before touching stdin/stdout.
func TestRunE_MissingToken(t *testing.T) {
	t.Setenv("NRFLO_MCP_TOKEN", "")
	agentMCPExternalCmd.SetContext(context.Background())
	t.Cleanup(func() { agentMCPExternalCmd.SetContext(context.TODO()) })
	if err := agentMCPExternalCmd.RunE(agentMCPExternalCmd, nil); err == nil {
		t.Fatal("expected an error when NRFLO_MCP_TOKEN is unset")
	}
}

// TestRunE_FailingSessionExchangeReturnsStatus covers case 8: a failing
// session exchange surfaces the HTTP status, not a silent tool-less server.
func TestRunE_FailingSessionExchangeReturnsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NRFLO_MCP_TOKEN", "tok")
	t.Setenv("NRFLO_SERVER_URL", srv.URL)
	t.Setenv("NRFLO_PROJECT", "p1")
	agentMCPExternalCmd.SetContext(context.Background())
	t.Cleanup(func() { agentMCPExternalCmd.SetContext(context.TODO()) })

	err := agentMCPExternalCmd.RunE(agentMCPExternalCmd, nil)
	if err == nil {
		t.Fatal("expected an error from a failing session exchange")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should surface the HTTP status, got: %v", err)
	}
}
