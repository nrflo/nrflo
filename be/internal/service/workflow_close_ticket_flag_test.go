package service

import (
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// boolPtr returns a pointer to a bool literal.
func boolPtr(b bool) *bool { return &b }

// --- CreateWorkflowDef: close_ticket_on_complete defaults ---

func TestCreateWorkflowDef_CloseTicketOnComplete_DefaultsToTrue(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	// Omit close_ticket_on_complete entirely → should default to true
	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-ctoc-default",
		Phases: phases,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if !wf.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=true (default), got false")
	}
}

func TestCreateWorkflowDef_CloseTicketOnComplete_ExplicitFalse(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-ctoc-false",
		Phases:                phases,
		CloseTicketOnComplete: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if wf.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=false, got true")
	}
}

func TestCreateWorkflowDef_CloseTicketOnComplete_ExplicitTrue(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-ctoc-true",
		Phases:                phases,
		CloseTicketOnComplete: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if !wf.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=true, got false")
	}
}

// --- GetWorkflowDef / ListWorkflowDefs: field persisted and read back ---

func TestGetWorkflowDef_CloseTicketOnComplete_Persisted(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	cases := []struct {
		id   string
		want bool
		req  *bool
	}{
		{"wf-get-ctoc-true", true, boolPtr(true)},
		{"wf-get-ctoc-false", false, boolPtr(false)},
		{"wf-get-ctoc-nil", true, nil}, // nil → default true
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
				ID:                    tc.id,
				Phases:                phases,
				CloseTicketOnComplete: tc.req,
			})
			if err != nil {
				t.Fatalf("CreateWorkflowDef: %v", err)
			}

			def, err := svc.GetWorkflowDef("proj1", tc.id)
			if err != nil {
				t.Fatalf("GetWorkflowDef: %v", err)
			}
			if def.CloseTicketOnComplete != tc.want {
				t.Errorf("GetWorkflowDef CloseTicketOnComplete: got %v, want %v", def.CloseTicketOnComplete, tc.want)
			}
		})
	}
}

func TestListWorkflowDefs_CloseTicketOnComplete_Persisted(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-list-ctoc",
		Phases:                phases,
		CloseTicketOnComplete: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	def, ok := defs["wf-list-ctoc"]
	if !ok {
		t.Fatal("wf-list-ctoc not found in result")
	}
	if def.CloseTicketOnComplete {
		t.Errorf("ListWorkflowDefs CloseTicketOnComplete: got true, want false")
	}
}

// --- UpdateWorkflowDef: field is updateable ---

func TestUpdateWorkflowDef_CloseTicketOnComplete_UpdateToFalse(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	// Create with default (true)
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-upd-ctoc",
		Phases: phases,
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}

	// Update to false
	if err := svc.UpdateWorkflowDef("proj1", "wf-upd-ctoc", &types.WorkflowDefUpdateRequest{
		CloseTicketOnComplete: boolPtr(false),
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-upd-ctoc")
	if err != nil {
		t.Fatalf("GetWorkflowDef after update: %v", err)
	}
	if def.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=false after update, got true")
	}
}

func TestUpdateWorkflowDef_CloseTicketOnComplete_UpdateToTrue(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	// Create with explicit false
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-upd-ctoc-tot",
		Phases:                phases,
		CloseTicketOnComplete: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}

	// Update back to true
	if err := svc.UpdateWorkflowDef("proj1", "wf-upd-ctoc-tot", &types.WorkflowDefUpdateRequest{
		CloseTicketOnComplete: boolPtr(true),
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-upd-ctoc-tot")
	if err != nil {
		t.Fatalf("GetWorkflowDef after update: %v", err)
	}
	if !def.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=true after update, got false")
	}
}

func TestUpdateWorkflowDef_CloseTicketOnComplete_NilPreservesValue(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                    "wf-upd-nil-ctoc",
		Phases:                phases,
		CloseTicketOnComplete: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}

	// Update description only — close_ticket_on_complete must be unchanged
	desc := "updated desc"
	if err := svc.UpdateWorkflowDef("proj1", "wf-upd-nil-ctoc", &types.WorkflowDefUpdateRequest{
		Description: &desc,
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-upd-nil-ctoc")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.CloseTicketOnComplete {
		t.Errorf("expected CloseTicketOnComplete=false (preserved), got true")
	}
}

// --- BuildSpawnerConfig: propagates close_ticket_on_complete ---

func TestBuildSpawnerConfig_CloseTicketOnComplete_Propagated(t *testing.T) {
	phasesJSON := `[{"agent":"analyzer","layer":0}]`

	cases := []struct {
		name  string
		value bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := &model.Workflow{
				ID:                    "wf-spawner",
				ProjectID:             "proj",
				ScopeType:             "ticket",
				CloseTicketOnComplete: tc.value,
				Phases:                phasesJSON,
				Groups:                "[]",
			}

			workflows, _ := BuildSpawnerConfig([]*model.Workflow{wf}, nil)
			def, ok := workflows["wf-spawner"]
			if !ok {
				t.Fatal("workflow not found in spawner config")
			}
			if def.CloseTicketOnComplete != tc.value {
				t.Errorf("BuildSpawnerConfig CloseTicketOnComplete: got %v, want %v", def.CloseTicketOnComplete, tc.value)
			}
		})
	}
}

// --- WorkflowDef MarshalJSON includes close_ticket_on_complete ---

func TestWorkflowDef_MarshalJSON_IncludesCloseTicketOnComplete(t *testing.T) {
	cases := []struct {
		name  string
		value bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := WorkflowDef{
				Description:           "test",
				ScopeType:             "ticket",
				CloseTicketOnComplete: tc.value,
				Groups:                []string{},
				Phases:                []PhaseDef{{ID: "a", Agent: "a", Layer: 0}},
			}

			b, err := json.Marshal(wf)
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			raw, ok := m["close_ticket_on_complete"]
			if !ok {
				t.Fatal("close_ticket_on_complete not present in marshaled JSON")
			}
			got, ok := raw.(bool)
			if !ok {
				t.Fatalf("close_ticket_on_complete is not bool: %T", raw)
			}
			if got != tc.value {
				t.Errorf("close_ticket_on_complete: got %v, want %v", got, tc.value)
			}
		})
	}
}
