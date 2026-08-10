package orchestrator

import (
	"encoding/json"

	"be/internal/spawner"
)

// resolveSessionSpawner finds the spawner serving a session's tool registry:
// the run's own spawners first, then the run-less auxiliary registrations
// (planner). Returns nil when neither knows the session.
func (o *Orchestrator) resolveSessionSpawner(instanceID, sessionID string) *spawner.Spawner {
	o.mu.Lock()
	defer o.mu.Unlock()
	if rs, ok := o.runs[instanceID]; ok {
		if sp := rs.spawners[sessionID]; sp != nil {
			return sp
		}
	}
	return o.auxSpawners[sessionID]
}

// RegisterAuxSpawner records a one-off child spawner so the socket bridge can
// serve its session's tools; UnregisterAuxSpawner is its symmetric teardown.
// Exported for host spawners built outside this package (api's refinery fold
// host) — a session missing from this index silently gets an empty tools/list
// and no heartbeat.
func (o *Orchestrator) RegisterAuxSpawner(sessionID string, sp *spawner.Spawner) {
	o.mu.Lock()
	o.auxSpawners[sessionID] = sp
	o.mu.Unlock()
}

func (o *Orchestrator) UnregisterAuxSpawner(sessionID string) {
	o.mu.Lock()
	delete(o.auxSpawners, sessionID)
	o.mu.Unlock()
}

// ListTools returns the MCP tools array for the given session inside an active workflow run.
// Returns an empty array when the run or session is not found (benign/no-op).
func (o *Orchestrator) ListTools(instanceID, sessionID string) (json.RawMessage, error) {
	sp := o.resolveSessionSpawner(instanceID, sessionID)
	if sp == nil {
		return json.RawMessage("[]"), nil
	}
	return sp.ListTools(sessionID)
}

// CallTool dispatches a tool call to the session's API-via-CLI registry.
// media is a marshaled media array (see spawner.toolMediaWire), nil when none.
// When the tool returns a terminal signal, RequestTerminalSignal is fired before returning.
func (o *Orchestrator) CallTool(instanceID, sessionID, name string, input json.RawMessage) (output string, media json.RawMessage, isError bool, err error) {
	sp := o.resolveSessionSpawner(instanceID, sessionID)
	if sp == nil {
		return "session not active", nil, true, nil
	}

	out, media, isErr, terminal, callErr := sp.DispatchTool(sessionID, name, input)
	if terminal != "" {
		sp.RequestTerminalSignal(sessionID, terminal)
	}
	return out, media, isErr, callErr
}
