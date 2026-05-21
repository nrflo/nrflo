package model

import (
	"encoding/json"
	"testing"
)

// TestWorkflowJSONRoundTrip verifies that fields tagged json:"-" but emitted by
// MarshalJSON survive a marshal→unmarshal cycle (workflow export/import path).
func TestWorkflowJSONRoundTrip(t *testing.T) {
	t.Parallel()
	w := Workflow{
		ID:                      "wf-1",
		ProjectID:               "p",
		Description:             "d",
		ScopeType:               "project",
		CloseTicketOnComplete:   true,
		NextWorkflowOnSuccess:   "wf-2",
		PauseEventCommand:       "on-pause",
		PauseEventScriptID:      "",
		FinalizeSuccessScriptID: "sid",
	}
	w.SetGroups([]string{"g1", "g2"})

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Workflow
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.PauseEventCommand != "on-pause" {
		t.Errorf("PauseEventCommand = %q, want %q", got.PauseEventCommand, "on-pause")
	}
	if got.NextWorkflowOnSuccess != "wf-2" {
		t.Errorf("NextWorkflowOnSuccess = %q, want %q", got.NextWorkflowOnSuccess, "wf-2")
	}
	if got.FinalizeSuccessScriptID != "sid" {
		t.Errorf("FinalizeSuccessScriptID = %q, want %q", got.FinalizeSuccessScriptID, "sid")
	}
	if g := got.GetGroups(); len(g) != 2 || g[0] != "g1" || g[1] != "g2" {
		t.Errorf("GetGroups() = %v, want [g1 g2]", g)
	}
}
