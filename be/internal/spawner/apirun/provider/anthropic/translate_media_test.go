package anthropic

import (
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestTranslateRequest_ContentBlocks_ToolResultMedia pins the wire shape:
// media travels as sibling blocks after the tool_result, never inside it
// (OpenRouter's anthropic passthrough rejects document parts on tool_result).
func TestTranslateRequest_ContentBlocks_ToolResultMedia(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "tool_doc",
				Output:    "Loaded chanote.pdf (application/pdf).",
				OutputMedia: []provider.MediaBlock{
					{Kind: "document", MediaType: "application/pdf", DataB64: "JVBERi0=", Name: "chanote.pdf"},
					{Kind: "image", MediaType: "image/png", DataB64: "iVBORw0KGgo="},
				},
			}},
		}},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	content := params.Messages[0].Content
	if len(content) != 4 {
		t.Fatalf("len(content) = %d, want 4 (tool_result, label, document, image)", len(content))
	}

	tr := content[0].OfToolResult
	if tr == nil || tr.ToolUseID != "tool_doc" {
		t.Fatalf("content[0] is not the tool_result: %+v", content[0])
	}
	for i, c := range tr.Content {
		if c.OfDocument != nil || c.OfImage != nil {
			t.Errorf("tool_result content[%d] carries media; media must be a sibling block", i)
		}
	}
	if len(tr.Content) != 1 || tr.Content[0].OfText == nil || tr.Content[0].OfText.Text != "Loaded chanote.pdf (application/pdf)." {
		t.Errorf("tool_result content = %+v, want the text output only", tr.Content)
	}

	if content[1].OfText == nil || !strings.Contains(content[1].OfText.Text, "tool_doc") {
		t.Errorf("content[1] = %+v, want a label naming the tool call", content[1])
	}
	doc := content[2].OfDocument
	if doc == nil || doc.Source.OfBase64 == nil || doc.Source.OfBase64.Data != "JVBERi0=" {
		t.Errorf("content[2] = %+v, want the pdf document block", content[2])
	}
	if doc != nil && doc.Title.Value != "chanote.pdf" {
		t.Errorf("document title = %q, want chanote.pdf", doc.Title.Value)
	}
	img := content[3].OfImage
	if img == nil || img.Source.OfBase64 == nil || img.Source.OfBase64.Data != "iVBORw0KGgo=" {
		t.Errorf("content[3] = %+v, want the png image block", content[3])
	}
}

// TestTranslateRequest_ContentBlocks_ToolResultMediaOrdering verifies that with
// several tool_results in one turn, every tool_result still leads the message
// and each media group is labelled with its own tool_use_id.
func TestTranslateRequest_ContentBlocks_ToolResultMediaOrdering(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{
				{
					Type: "tool_result", ToolUseID: "tool_a", Output: "a",
					OutputMedia: []provider.MediaBlock{{Kind: "document", MediaType: "application/pdf", DataB64: "QQ=="}},
				},
				{Type: "tool_result", ToolUseID: "tool_plain", Output: "plain"},
				{
					Type: "tool_result", ToolUseID: "tool_b", Output: "b",
					OutputMedia: []provider.MediaBlock{{Kind: "image", MediaType: "image/png", DataB64: "Qg=="}},
				},
			},
		}},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	content := params.Messages[0].Content
	if len(content) != 7 {
		t.Fatalf("len(content) = %d, want 7 (3 tool_results + 2 label/media pairs)", len(content))
	}
	for i, want := range []string{"tool_a", "tool_plain", "tool_b"} {
		if content[i].OfToolResult == nil || content[i].OfToolResult.ToolUseID != want {
			t.Fatalf("content[%d] = %+v, want tool_result %s leading the turn", i, content[i], want)
		}
	}
	if content[3].OfText == nil || !strings.Contains(content[3].OfText.Text, "tool_a") {
		t.Errorf("content[3] = %+v, want tool_a label", content[3])
	}
	if content[4].OfDocument == nil {
		t.Errorf("content[4] = %+v, want tool_a document", content[4])
	}
	if content[5].OfText == nil || !strings.Contains(content[5].OfText.Text, "tool_b") {
		t.Errorf("content[5] = %+v, want tool_b label", content[5])
	}
	if content[6].OfImage == nil {
		t.Errorf("content[6] = %+v, want tool_b image", content[6])
	}
}

func TestTranslateRequest_ContentBlocks_ToolResultMediaBadType(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:        "tool_result",
				ToolUseID:   "tool_bad",
				OutputMedia: []provider.MediaBlock{{Kind: "image", MediaType: "image/tiff", DataB64: "AA=="}},
			}},
		}},
	}
	_, err := translateRequest(req)
	if err == nil || !strings.Contains(err.Error(), "unsupported image media type") {
		t.Fatalf("err = %v, want unsupported image media type", err)
	}
}
