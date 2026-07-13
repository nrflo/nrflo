package service

import (
	"encoding/json"
	"fmt"

	"be/internal/repo"
	"be/internal/types"
)

// Add adds a finding to the current agent session
func (s *FindingsService) Add(req *types.FindingsAddRequest) (BroadcastCtx, error) {
	if err := GuardReservedFindingKey(req.Key); err != nil {
		return BroadcastCtx{}, err
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Upsert("session", req.SessionID, req.Key, val, denorm, actor)
}

// AddBulk adds multiple findings to the current agent session in one operation
func (s *FindingsService) AddBulk(req *types.FindingsAddBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}
	if err := GuardReservedFindingKeys(req.KeyValues); err != nil {
		return BroadcastCtx{}, err
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	for key, value := range req.KeyValues {
		val := json.RawMessage(normalizeJSONValue(value))
		if err := s.findingRepo.Upsert("session", req.SessionID, key, val, denorm, actor); err != nil {
			return BroadcastCtx{}, err
		}
	}
	return bctx, nil
}

// Append appends a value to an existing finding (creating array if needed)
func (s *FindingsService) Append(req *types.FindingsAppendRequest) (BroadcastCtx, error) {
	if req.Key == "" {
		return BroadcastCtx{}, fmt.Errorf("key is required")
	}
	if err := GuardReservedFindingKey(req.Key); err != nil {
		return BroadcastCtx{}, err
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	return bctx, s.findingRepo.Append("session", req.SessionID, req.Key, val, denorm, actor)
}

// AppendBulk appends multiple values at once
func (s *FindingsService) AppendBulk(req *types.FindingsAppendBulkRequest) (BroadcastCtx, error) {
	if len(req.KeyValues) == 0 {
		return BroadcastCtx{}, fmt.Errorf("at least one key-value pair is required")
	}
	if err := GuardReservedFindingKeys(req.KeyValues); err != nil {
		return BroadcastCtx{}, err
	}
	bctx, denorm, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, err
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	for key, value := range req.KeyValues {
		val := json.RawMessage(normalizeJSONValue(value))
		if err := s.findingRepo.Append("session", req.SessionID, key, val, denorm, actor); err != nil {
			return BroadcastCtx{}, err
		}
	}
	return bctx, nil
}

// Delete removes finding keys from the current agent session
func (s *FindingsService) Delete(req *types.FindingsDeleteRequest) (BroadcastCtx, int, error) {
	if len(req.Keys) == 0 {
		return BroadcastCtx{}, 0, fmt.Errorf("at least one key is required")
	}
	bctx, _, err := s.loadSessionContext(req.SessionID)
	if err != nil {
		return BroadcastCtx{}, 0, nil // No session = nothing to delete
	}
	actor := repo.Actor{ID: req.SessionID, Source: "agent"}
	deleted, err := s.findingRepo.DeleteKeys("session", req.SessionID, req.Keys, actor)
	if err != nil {
		return BroadcastCtx{}, 0, err
	}
	return bctx, len(deleted), nil
}
