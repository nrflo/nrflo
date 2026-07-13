package socket

// tools.call media passthrough tests — split from handler_tools_test.go to
// stay under 300 lines.

import (
	"encoding/json"
	"testing"
)

func TestHandleTools_Call_MediaIncluded(t *testing.T) {
	td := &fakeToolDispatcher{
		callOutput: "loaded",
		callMedia:  json.RawMessage(`[{"kind":"image","media_type":"image/png","data_b64":"aGk=","name":"scan.png"}]`),
	}
	h := &Handler{toolDispatcher: td}

	resp := h.handleTools(buildToolsReq("r1", "call", `{"session_id":"s","instance_id":"i","name":"read_document","input":{}}`), "call")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result struct {
		Output string `json:"output"`
		Media  []struct {
			Kind      string `json:"kind"`
			MediaType string `json:"media_type"`
			DataB64   string `json:"data_b64"`
		} `json:"media"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Media) != 1 || result.Media[0].Kind != "image" || result.Media[0].DataB64 != "aGk=" {
		t.Errorf("media = %+v", result.Media)
	}
}

func TestHandleTools_Call_NoMediaOmitted(t *testing.T) {
	td := &fakeToolDispatcher{callOutput: "plain"}
	h := &Handler{toolDispatcher: td}

	resp := h.handleTools(buildToolsReq("r2", "call", `{"session_id":"s","instance_id":"i","name":"t","input":{}}`), "call")
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, ok := result["media"]; ok {
		t.Errorf("media key should be omitted when nil; result=%v", result)
	}
}
