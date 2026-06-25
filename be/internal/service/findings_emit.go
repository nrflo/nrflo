package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"be/internal/repo"
	"be/internal/types"
)

// Emit validates a finding against the workflow-scoped schema registered for
// its key, then stores it on the current agent session. On a validation failure
// (or an unknown key) it returns an error whose message is meant to be shown to
// the agent as a tool result so it can correct the value and retry; nothing is
// stored in that case.
func (s *FindingsService) Emit(req *types.FindingsEmitRequest) (BroadcastCtx, error) {
	if strings.TrimSpace(req.Key) == "" {
		return BroadcastCtx{}, fmt.Errorf("key is required")
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}

	defs, err := loadFindingSchemas(s.pool, bctx.ProjectID, bctx.Workflow)
	if err != nil {
		return BroadcastCtx{}, err
	}

	var def *types.FindingSchema
	for i := range defs {
		if defs[i].Key == req.Key {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		return BroadcastCtx{}, fmt.Errorf("no schema defined for key '%s'. Configured keys: %s", req.Key, configuredKeys(defs))
	}

	sch, err := compileJSONSchema(string(def.Schema))
	if err != nil {
		return BroadcastCtx{}, fmt.Errorf("schema for key '%s' is invalid: %w", req.Key, err)
	}

	// Tolerate clients that double-encode the value as a JSON string containing
	// JSON (some CLI MCP bridges stringify object/array tool arguments, so an
	// object arrives as "{\"q\":1}" instead of {"q":1}). A genuine scalar string
	// is left untouched, so real string findings still validate as-is.
	rawValue := unwrapDoubleEncodedJSON(req.Value)

	var value interface{}
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return BroadcastCtx{}, fmt.Errorf("value for key '%s' is not valid JSON: %v\nExpected structure example:\n%s", req.Key, err, def.Example)
	}
	if err := sch.Validate(value); err != nil {
		return BroadcastCtx{}, fmt.Errorf("value for key '%s' does not match the required structure: %v\nExpected structure example:\n%s", req.Key, err, def.Example)
	}

	val := json.RawMessage(normalizeJSONValue(string(rawValue)))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Upsert("session", req.SessionID, req.Key, val, denorm, actor)
}

// unwrapDoubleEncodedJSON unwraps a value that was serialized as a JSON string
// whose contents are themselves a JSON object or array — a quirk of some CLI MCP
// clients that stringify structured tool arguments (e.g. "{\"q\":1}" instead of
// {"q":1}). It unwraps ONE level and ONLY when the decoded contents are a JSON
// object/array; scalar strings, and values that are already structured, are
// returned unchanged, so legitimate string findings are never altered.
func unwrapDoubleEncodedJSON(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return raw // not a JSON string
	}
	var inner string
	if err := json.Unmarshal(raw, &inner); err != nil {
		return raw
	}
	innerTrim := strings.TrimSpace(inner)
	if len(innerTrim) == 0 || (innerTrim[0] != '{' && innerTrim[0] != '[') {
		return raw // inner is not an object/array → keep original (real string value)
	}
	if !json.Valid([]byte(innerTrim)) {
		return raw // inner only looks like JSON → keep original
	}
	return json.RawMessage(innerTrim)
}

// configuredKeys returns a comma-separated sorted list of schema keys, or a
// placeholder when none are defined.
func configuredKeys(defs []types.FindingSchema) string {
	if len(defs) == 0 {
		return "(none defined for this workflow)"
	}
	keys := make([]string, 0, len(defs))
	for _, d := range defs {
		keys = append(keys, d.Key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
