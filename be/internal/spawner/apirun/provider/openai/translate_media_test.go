package openai

import (
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

func toolResultReq(media []provider.MediaBlock) provider.Request {
	return provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:        "tool_result",
				ToolUseID:   "call_1",
				Output:      "Loaded doc.",
				OutputMedia: media,
			}},
		}},
	}
}

func TestTranslateRequest_ToolResultImageMedia(t *testing.T) {
	body := string(marshaledParams(t, toolResultReq([]provider.MediaBlock{
		{Kind: "image", MediaType: "image/png", DataB64: "aGVsbG8=", Name: "scan.png"},
	})))
	if !strings.Contains(body, `"function_call_output"`) {
		t.Errorf("function_call_output missing; body=%s", body)
	}
	if !strings.Contains(body, `"input_image"`) {
		t.Errorf("input_image part missing; body=%s", body)
	}
	if !strings.Contains(body, `"image_url":"data:image/png;base64,aGVsbG8="`) {
		t.Errorf("base64 data URL missing; body=%s", body)
	}
	if !strings.Contains(body, `"detail":"high"`) {
		t.Errorf("detail=high missing; body=%s", body)
	}
	if !strings.Contains(body, `Media returned by tool call call_1`) {
		t.Errorf("anchor text missing; body=%s", body)
	}
}

func TestTranslateRequest_ToolResultPDFMedia(t *testing.T) {
	body := string(marshaledParams(t, toolResultReq([]provider.MediaBlock{
		{Kind: "document", MediaType: "application/pdf", DataB64: "cGRm", Name: "deed.pdf"},
	})))
	if !strings.Contains(body, `"input_file"`) {
		t.Errorf("input_file part missing; body=%s", body)
	}
	if !strings.Contains(body, `"file_data":"data:application/pdf;base64,cGRm"`) {
		t.Errorf("file_data data URL missing; body=%s", body)
	}
	if !strings.Contains(body, `"filename":"deed.pdf"`) {
		t.Errorf("filename missing; body=%s", body)
	}
	if !strings.Contains(body, `"detail":"high"`) {
		t.Errorf("input_file detail=high missing; body=%s", body)
	}
}

func TestTranslateRequest_ToolResultPDFMedia_DefaultFilename(t *testing.T) {
	body := string(marshaledParams(t, toolResultReq([]provider.MediaBlock{
		{Kind: "document", MediaType: "application/pdf", DataB64: "cGRm"},
	})))
	if !strings.Contains(body, `"filename":"document.pdf"`) {
		t.Errorf("default filename missing; body=%s", body)
	}
}

func TestTranslateRequest_ToolResultMedia_UnsupportedType(t *testing.T) {
	if _, err := translateRequest(toolResultReq([]provider.MediaBlock{
		{Kind: "image", MediaType: "image/tiff", DataB64: "eA=="},
	})); err == nil {
		t.Fatalf("expected error for unsupported image media type")
	}
	if _, err := translateRequest(toolResultReq([]provider.MediaBlock{
		{Kind: "document", MediaType: "text/csv", DataB64: "eA=="},
	})); err == nil {
		t.Fatalf("expected error for unsupported document media type")
	}
}

func TestTranslateRequest_ToolResultNoMedia_NoFollowup(t *testing.T) {
	body := string(marshaledParams(t, toolResultReq(nil)))
	if strings.Contains(body, `"input_image"`) || strings.Contains(body, `"input_file"`) {
		t.Errorf("unexpected media parts without OutputMedia; body=%s", body)
	}
	if strings.Contains(body, "Media returned by tool call") {
		t.Errorf("unexpected media follow-up message; body=%s", body)
	}
}
