package spawner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"be/internal/spawner/apirun"
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

// DispatchTool invokes the named tool handler for the session.
// terminal is non-empty when the tool returned a TerminalSignal (e.g. "pass", "fail");
// in that case the caller should fire RequestTerminalSignal.
func (s *Spawner) DispatchTool(sessionID, name string, input json.RawMessage) (output string, isError bool, terminal string, err error) {
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return fmt.Sprintf("unknown session: %s", sessionID), true, "", nil
	}
	handler, ok := proc.apiHandlers[name]
	if !ok {
		return fmt.Sprintf("unknown tool: %s", name), true, "", nil
	}

	var (
		out   string
		isErr bool
		terr  error
	)
	if mh, ok2 := handler.(apirun.MediaToolHandler); ok2 {
		out, _, isErr, terr = mh.InvokeMedia(context.Background(), proc.apiToolEnv, input)
	} else {
		out, isErr, terr = handler.Invoke(context.Background(), proc.apiToolEnv, input)
	}

	var ts apirun.TerminalSignal
	if errors.As(terr, &ts) {
		return "", false, strings.ToLower(ts.Status), nil
	}
	if terr != nil {
		return terr.Error(), true, "", nil
	}
	return out, isErr, "", nil
}
