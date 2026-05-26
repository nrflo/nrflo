package orchestrator

import (
	"encoding/json"
)

// ListTools returns the MCP tools array for the given session inside an active workflow run.
// Returns an empty array when the run or session is not found (benign/no-op).
func (o *Orchestrator) ListTools(instanceID, sessionID string) (json.RawMessage, error) {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	if !ok {
		o.mu.Unlock()
		return json.RawMessage("[]"), nil
	}
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return json.RawMessage("[]"), nil
	}
	return sp.ListTools(sessionID)
}

// CallTool dispatches a tool call to the session's API-via-CLI registry.
// When the tool returns a terminal signal, RequestTerminalSignal is fired before returning.
func (o *Orchestrator) CallTool(instanceID, sessionID, name string, input json.RawMessage) (output string, isError bool, err error) {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	if !ok {
		o.mu.Unlock()
		return "workflow run not found", true, nil
	}
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return "session not active", true, nil
	}

	out, isErr, terminal, callErr := sp.DispatchTool(sessionID, name, input)
	if terminal != "" {
		sp.RequestTerminalSignal(sessionID, terminal)
	}
	return out, isErr, callErr
}
