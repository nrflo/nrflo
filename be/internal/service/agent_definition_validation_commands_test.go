package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/types"
)

// --- Create: happy path ---

func TestCreateAgentDef_ValidationCommands_ValidPersists(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	cmds := []string{"true", "make test"}
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                 "vc-valid",
		Prompt:             "do stuff",
		ValidationCommands: &cmds,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with valid commands: %v", err)
	}

	var got []string
	if err := json.Unmarshal([]byte(def.ValidationCommands), &got); err != nil {
		t.Fatalf("unmarshal returned ValidationCommands: %v", err)
	}
	if len(got) != 2 || got[0] != "true" || got[1] != "make test" {
		t.Errorf("ValidationCommands = %v, want [true make test]", got)
	}
}

func TestCreateAgentDef_ValidationCommands_RoundTripViaGet(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	cmds := []string{"true", "make test"}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                 "vc-rtrip",
		Prompt:             "do stuff",
		ValidationCommands: &cmds,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "vc-rtrip")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}

	var got []string
	if err := json.Unmarshal([]byte(def.ValidationCommands), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0] != "true" || got[1] != "make test" {
		t.Errorf("Get ValidationCommands = %v, want [true make test]", got)
	}
}

func TestCreateAgentDef_ValidationCommands_RoundTripViaList(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	cmds := []string{"make lint"}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                 "vc-list",
		Prompt:             "do stuff",
		ValidationCommands: &cmds,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}

	var got []string
	if err := json.Unmarshal([]byte(defs[0].ValidationCommands), &got); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if len(got) != 1 || got[0] != "make lint" {
		t.Errorf("List ValidationCommands = %v, want [make lint]", got)
	}
}

// --- Create: omit field stores empty array ---

func TestCreateAgentDef_ValidationCommands_OmittedStoresEmptyArray(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "vc-omit",
		Prompt: "do stuff",
		// ValidationCommands intentionally nil
	})
	if err != nil {
		t.Fatalf("CreateAgentDef without commands: %v", err)
	}

	if def.ValidationCommands != "[]" {
		t.Errorf("ValidationCommands = %q, want %q", def.ValidationCommands, "[]")
	}

	got, err := svc.GetAgentDef("proj1", wfID, "vc-omit")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.ValidationCommands != "[]" {
		t.Errorf("Get ValidationCommands = %q, want %q", got.ValidationCommands, "[]")
	}
}

// --- Create: validation rejections ---

func TestCreateAgentDef_ValidationCommands_InvalidCases(t *testing.T) {
	t.Parallel()

	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = "true"
	}

	longEntry := strings.Repeat("x", 1025)

	tests := []struct {
		name string
		cmds []string
	}{
		{"empty_string_entry", []string{""}},
		{"whitespace_only_entry", []string{"   "}},
		{"tab_only_entry", []string{"\t"}},
		{"too_many_entries_21", tooMany},
		{"entry_exceeds_1024_bytes", []string{longEntry}},
	}

	for i, tt := range tests {
		tt := tt
		i := i
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, svc, wfID := setupAgentDefTestEnv(t, nil)

			cmds := tt.cmds
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                 "vc-inv-" + string(rune('a'+i)),
				Prompt:             "do stuff",
				ValidationCommands: &cmds,
			})
			if err == nil {
				t.Errorf("CreateAgentDef(%s): want error, got nil", tt.name)
			}
		})
	}
}

// --- Create: boundary: exactly 20 entries and exactly 1024-byte entry accepted ---

func TestCreateAgentDef_ValidationCommands_BoundaryAccepted(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	// Exactly 20 entries.
	cmds20 := make([]string, 20)
	for i := range cmds20 {
		cmds20[i] = "true"
	}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                 "vc-20",
		Prompt:             "do stuff",
		ValidationCommands: &cmds20,
	}); err != nil {
		t.Errorf("CreateAgentDef with 20 entries: %v", err)
	}

	// Exactly 1024-byte entry.
	exact1024 := []string{strings.Repeat("x", 1024)}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                 "vc-1024",
		Prompt:             "do stuff",
		ValidationCommands: &exact1024,
	}); err != nil {
		t.Errorf("CreateAgentDef with 1024-byte entry: %v", err)
	}
}

// --- Update: valid commands ---

func TestUpdateAgentDef_ValidationCommands_Valid(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "vc-upd",
		Prompt: "do stuff",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newCmds := []string{"make test", "make lint"}
	if err := svc.UpdateAgentDef("proj1", wfID, "vc-upd", &types.AgentDefUpdateRequest{
		ValidationCommands: &newCmds,
	}); err != nil {
		t.Fatalf("UpdateAgentDef with valid commands: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "vc-upd")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	var got []string
	if err := json.Unmarshal([]byte(def.ValidationCommands), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0] != "make test" || got[1] != "make lint" {
		t.Errorf("after update ValidationCommands = %v, want [make test make lint]", got)
	}
}

// --- Update: invalid commands rejected ---

func TestUpdateAgentDef_ValidationCommands_InvalidCases(t *testing.T) {
	t.Parallel()

	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = "true"
	}

	tests := []struct {
		name string
		cmds []string
	}{
		{"empty_string_entry", []string{""}},
		{"whitespace_only_entry", []string{"  "}},
		{"too_many_entries_21", tooMany},
		{"entry_exceeds_1024_bytes", []string{strings.Repeat("y", 1025)}},
	}

	for i, tt := range tests {
		tt := tt
		i := i
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, svc, wfID := setupAgentDefTestEnv(t, nil)

			if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:     "vc-upd-inv-" + string(rune('a'+i)),
				Prompt: "do stuff",
			}); err != nil {
				t.Fatalf("create: %v", err)
			}

			cmds := tt.cmds
			if err := svc.UpdateAgentDef("proj1", wfID, "vc-upd-inv-"+string(rune('a'+i)), &types.AgentDefUpdateRequest{
				ValidationCommands: &cmds,
			}); err == nil {
				t.Errorf("UpdateAgentDef(%s): want error, got nil", tt.name)
			}
		})
	}
}
