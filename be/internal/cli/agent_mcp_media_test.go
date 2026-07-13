package cli

// tools/call media tests — image media blocks from the socket result become
// MCP image content blocks; non-image kinds are dropped.

import (
	"encoding/json"
	"testing"
)

func callWithMedia(t *testing.T, socketResult string) map[string]interface{} {
	t.Helper()
	caller := &fakeMCPCaller{toolsCallResult: json.RawMessage(socketResult)}
	resp := dispatchMCP(makeMCPReq(1, "tools/call", `{"name":"read_document","arguments":{"name":"a"}}`), "s", "i", false, caller)
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected response: %+v", resp)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}
	return result
}

func TestDispatchMCP_ToolsCall_ImageMedia(t *testing.T) {
	result := callWithMedia(t,
		`{"output":"loaded","is_error":false,"media":[{"kind":"image","media_type":"image/png","data_b64":"aGk=","name":"scan.png"}]}`)
	content, _ := result["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content len = %d, want 2 (text + image)", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "loaded" {
		t.Errorf("content[0] = %v", content[0])
	}
	if content[1]["type"] != "image" || content[1]["data"] != "aGk=" || content[1]["mimeType"] != "image/png" {
		t.Errorf("content[1] = %v", content[1])
	}
}

func TestDispatchMCP_ToolsCall_NonImageMediaDropped(t *testing.T) {
	result := callWithMedia(t,
		`{"output":"loaded","is_error":false,"media":[{"kind":"document","media_type":"application/pdf","data_b64":"cGRm"}]}`)
	content, _ := result["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1 (text only)", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("content[0] = %v", content[0])
	}
}

func TestDispatchMCP_ToolsCall_NoMediaField(t *testing.T) {
	result := callWithMedia(t, `{"output":"plain","is_error":false}`)
	content, _ := result["content"].([]map[string]interface{})
	if len(content) != 1 || content[0]["text"] != "plain" {
		t.Errorf("content = %v", content)
	}
}
