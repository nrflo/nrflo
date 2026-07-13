package openai

import (
	"fmt"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"

	"be/internal/spawner/apirun/provider"
)

// mediaFollowupItem builds the user-role input message carrying a tool_result's
// OutputMedia. The Responses API rejects image/file parts on
// function_call_output (its output is a plain string), so media travels as a
// separate user message appended right after the output item.
func mediaFollowupItem(b provider.ContentBlock) (responses.ResponseInputItemUnionParam, error) {
	content := responses.ResponseInputMessageContentListParam{
		responses.ResponseInputContentParamOfInputText("Media returned by tool call " + b.ToolUseID + ":"),
	}
	for _, m := range b.OutputMedia {
		part, err := translateMediaBlock(m)
		if err != nil {
			return responses.ResponseInputItemUnionParam{}, fmt.Errorf("tool_result %s: %w", b.ToolUseID, err)
		}
		content = append(content, part)
	}
	return responses.ResponseInputItemParamOfInputMessage(content, "user"), nil
}

// translateMediaBlock maps a provider.MediaBlock to a Responses input content
// part: images become input_image (base64 data URL, detail=high so scanned
// pages are not downsampled), PDFs become input_file (file_data data URL).
func translateMediaBlock(m provider.MediaBlock) (responses.ResponseInputContentUnionParam, error) {
	switch m.Kind {
	case "image":
		switch m.MediaType {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return responses.ResponseInputContentUnionParam{}, fmt.Errorf("unsupported image media type: %q", m.MediaType)
		}
		part := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailHigh)
		part.OfInputImage.ImageURL = param.NewOpt("data:" + m.MediaType + ";base64," + m.DataB64)
		return part, nil
	case "document":
		if m.MediaType != "application/pdf" {
			return responses.ResponseInputContentUnionParam{}, fmt.Errorf("unsupported document media type: %q", m.MediaType)
		}
		name := m.Name
		if name == "" {
			name = "document.pdf"
		}
		return responses.ResponseInputContentUnionParam{
			OfInputFile: &responses.ResponseInputFileParam{
				Filename: param.NewOpt(name),
				FileData: param.NewOpt("data:application/pdf;base64," + m.DataB64),
			},
		}, nil
	default:
		return responses.ResponseInputContentUnionParam{}, fmt.Errorf("unsupported media kind: %q", m.Kind)
	}
}
