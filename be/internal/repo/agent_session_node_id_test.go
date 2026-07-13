package repo

import (
	"testing"

	"be/internal/model"
)

// TestAgentSession_NodeID_RoundTripsCreateGet verifies NodeID survives
// Create then Get, independent of the (possibly different) Phase/AgentType values.
func TestAgentSession_NodeID_RoundTripsCreateGet(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupTestDB(t)

	session := &model.AgentSession{
		ID:                 "sess-nodeid-1",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		NodeID:             "analyzer-2",
		AgentType:          "setup-analyzer",
		Status:             model.AgentSessionRunning,
	}
	if err := r.Create(session); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-nodeid-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.NodeID != "analyzer-2" {
		t.Errorf("NodeID = %q, want analyzer-2", got.NodeID)
	}
	if got.Phase != "analyzer" {
		t.Errorf("Phase = %q, want analyzer (NodeID must not shadow Phase)", got.Phase)
	}
	if got.AgentType != "setup-analyzer" {
		t.Errorf("AgentType = %q, want setup-analyzer (NodeID must not shadow AgentType)", got.AgentType)
	}
}

// TestAgentSession_NodeID_DefaultsEmpty verifies that a session created
// without an explicit NodeID stores the column's NOT NULL DEFAULT ” rather
// than erroring or falling back to Phase.
func TestAgentSession_NodeID_DefaultsEmpty(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupTestDB(t)

	session := &model.AgentSession{
		ID:                 "sess-nodeid-2",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionRunning,
	}
	if err := r.Create(session); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-nodeid-2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("NodeID = %q, want empty string when unset on Create", got.NodeID)
	}
}

// TestAgentSession_NodeID_RoundTripsScanSessionJoined verifies NodeID survives
// through the JOINed query path (GetRunning -> scanSessionJoined), not just
// the simple Get path (scanSession).
func TestAgentSession_NodeID_RoundTripsScanSessionJoined(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupTestDB(t)

	session := &model.AgentSession{
		ID:                 "sess-nodeid-3",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		NodeID:             "builder-fanout-1",
		AgentType:          "implementor",
		Status:             model.AgentSessionRunning,
	}
	if err := r.Create(session); err != nil {
		t.Fatalf("Create: %v", err)
	}

	running, err := r.GetRunning(10)
	if err != nil {
		t.Fatalf("GetRunning: %v", err)
	}
	var found *model.AgentSession
	for _, s := range running {
		if s.ID == "sess-nodeid-3" {
			found = s
		}
	}
	if found == nil {
		t.Fatalf("sess-nodeid-3 not found in GetRunning results")
	}
	if found.NodeID != "builder-fanout-1" {
		t.Errorf("NodeID via scanSessionJoined = %q, want builder-fanout-1", found.NodeID)
	}
}
