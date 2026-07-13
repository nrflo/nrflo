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
// media is a marshaled media array (see spawner.toolMediaWire), nil when none.
// When the tool returns a terminal signal, RequestTerminalSignal is fired before returning.
func (o *Orchestrator) CallTool(instanceID, sessionID, name string, input json.RawMessage) (output string, media json.RawMessage, isError bool, err error) {
	o.mu.Lock()
	rs, ok := o.runs[instanceID]
	if !ok {
		o.mu.Unlock()
		return "workflow run not found", nil, true, nil
	}
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return "session not active", nil, true, nil
	}

	out, media, isErr, terminal, callErr := sp.DispatchTool(sessionID, name, input)
	if terminal != "" {
		sp.RequestTerminalSignal(sessionID, terminal)
	}
	return out, media, isErr, callErr
}
