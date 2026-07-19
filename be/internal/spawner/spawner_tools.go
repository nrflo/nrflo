package spawner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ListTools returns the MCP tools array for the session's API-via-CLI registry.
// Returns an empty JSON array when the session is not found or has no tools.
func (s *Spawner) ListTools(sessionID string) (json.RawMessage, error) {
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return json.RawMessage("[]"), nil
	}
	type toolEntry struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	entries := make([]toolEntry, 0, len(proc.apiTools))
	for _, spec := range proc.apiTools {
		entries = append(entries, toolEntry{
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: spec.InputSchema,
		})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal tools: %w", err)
	}
	return data, nil
}

// toolMediaWire is the JSON shape tool media travels in: the tools.call
// socket response's "media" array, parsed by the MCP bridge into image
// content blocks.
type toolMediaWire struct {
	Kind      string `json:"kind"`
	MediaType string `json:"media_type"`
	DataB64   string `json:"data_b64"`
	Name      string `json:"name,omitempty"`
}

// DispatchTool invokes the named tool handler for the session.
// media is a marshaled []toolMediaWire (nil when the tool returned none).
// terminal is non-empty when the tool returned a TerminalSignal (e.g. "pass", "fail");
// in that case the caller should fire RequestTerminalSignal.
func (s *Spawner) DispatchTool(sessionID, name string, input json.RawMessage) (output string, media json.RawMessage, isError bool, terminal string, err error) {
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return fmt.Sprintf("unknown session: %s", sessionID), nil, true, "", nil
	}
	handler, ok := proc.apiHandlers[name]
	if !ok {
		return fmt.Sprintf("unknown tool: %s", name), nil, true, "", nil
	}

	var (
		out    string
		blocks []provider.MediaBlock
		isErr  bool
		terr   error
	)
	if mh, ok2 := handler.(apirun.MediaToolHandler); ok2 {
		out, blocks, isErr, terr = mh.InvokeMedia(context.Background(), proc.apiToolEnv, input)
	} else {
		out, isErr, terr = handler.Invoke(context.Background(), proc.apiToolEnv, input)
	}

	var ts apirun.TerminalSignal
	if errors.As(terr, &ts) {
		return "", nil, false, strings.ToLower(ts.Status), nil
	}
	if terr != nil {
		return terr.Error(), nil, true, "", nil
	}
	if len(blocks) > 0 {
		wire := make([]toolMediaWire, 0, len(blocks))
		for _, b := range blocks {
			wire = append(wire, toolMediaWire{Kind: b.Kind, MediaType: b.MediaType, DataB64: b.DataB64, Name: b.Name})
		}
		if data, mErr := json.Marshal(wire); mErr == nil {
			media = data
		}
	}
	if !isErr {
		out = apirun.MaybeOffloadToolResult(context.Background(), proc.apiToolEnv, name, out)
	}
	return out, media, isErr, "", nil
}
