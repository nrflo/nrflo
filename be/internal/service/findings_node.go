package service

import "fmt"

// getByNode returns findings for a single execution node (read-time
// attribution via repo.GetByNode). An unknown node_id (no session for it in
// this instance) returns an error so callers can tell it apart from a known
// node that simply has no findings yet, which returns an empty map with a
// nil error.
func (s *FindingsService) getByNode(instanceID, nodeID string, keys []string) (interface{}, error) {
	wfiID, err := s.resolveWorkflowInstance(instanceID)
	if err != nil {
		return nil, err
	}
	raw, exists, err := s.findingRepo.GetByNode(wfiID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("node findings query failed: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("node '%s' not found", nodeID)
	}
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	return s.extractKeys(rawToInterface(raw), keys)
}
