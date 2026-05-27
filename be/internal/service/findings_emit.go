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

	var value interface{}
	if err := json.Unmarshal(req.Value, &value); err != nil {
		return BroadcastCtx{}, fmt.Errorf("value for key '%s' is not valid JSON: %v\nExpected structure example:\n%s", req.Key, err, def.Example)
	}
	if err := sch.Validate(value); err != nil {
		return BroadcastCtx{}, fmt.Errorf("value for key '%s' does not match the required structure: %v\nExpected structure example:\n%s", req.Key, err, def.Example)
	}

	val := json.RawMessage(normalizeJSONValue(string(req.Value)))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Upsert("session", req.SessionID, req.Key, val, denorm, actor)
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
