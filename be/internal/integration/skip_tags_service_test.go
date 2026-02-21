// Integration tests for skip-tags feature: JSON marshaling and service config.
package integration

import (
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

// TestMigration030MarshalJSON verifies that new fields appear in JSON output.
func TestMigration030MarshalJSON(t *testing.T) {
	t.Run("Workflow groups field present", func(t *testing.T) {
		wf := &model.Workflow{
			ID:        "test-wf",
			ProjectID: "proj",
			ScopeType: "ticket",
			Phases:    `[{"agent":"analyzer","layer":0}]`,
		}
		wf.SetGroups([]string{"be", "fe"})

		data, err := json.Marshal(wf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		groupsRaw, exists := result["groups"]
		if !exists {
			t.Fatalf("groups missing from Workflow JSON: %s", string(data))
		}
		groupsArr, ok := groupsRaw.([]interface{})
		if !ok || len(groupsArr) != 2 {
			t.Errorf("groups = %v, want [be fe]", groupsRaw)
		}
	})

	t.Run("Workflow empty groups is array not null", func(t *testing.T) {
		wf := &model.Workflow{ID: "wf-empty", ProjectID: "proj", ScopeType: "ticket", Phases: "[]", Groups: ""}
		data, _ := json.Marshal(wf)

		var result map[string]interface{}
		json.Unmarshal(data, &result)

		groupsRaw, exists := result["groups"]
		if !exists {
			t.Fatalf("groups missing (should be []): %s", string(data))
		}
		arr, ok := groupsRaw.([]interface{})
		if !ok || len(arr) != 0 {
			t.Errorf("groups = %v, want []", groupsRaw)
		}
	})

	t.Run("WorkflowInstance skip_tags field present", func(t *testing.T) {
		env := NewTestEnv(t)
		env.CreateTicket(t, "ST030-JSON-WFI", "marshal test")
		env.InitWorkflow(t, "ST030-JSON-WFI")

		wfi, err := env.WorkflowSvc.GetWorkflowInstance(env.ProjectID, "ST030-JSON-WFI", "test")
		if err != nil {
			t.Fatalf("GetWorkflowInstance: %v", err)
		}

		data, err := json.Marshal(wfi)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		skipTagsRaw, exists := result["skip_tags"]
		if !exists {
			t.Fatalf("skip_tags missing from WorkflowInstance JSON: %s", string(data))
		}
		arr, ok := skipTagsRaw.([]interface{})
		if !ok || len(arr) != 0 {
			t.Errorf("skip_tags = %v, want [] for fresh instance", skipTagsRaw)
		}
	})
}

// TestMigration030BuildSpawnerConfig verifies BuildSpawnerConfig populates
// Groups and Tag correctly from model objects.
func TestMigration030BuildSpawnerConfig(t *testing.T) {
	t.Run("Groups and Tag populated", func(t *testing.T) {
		wf := &model.Workflow{
			ID:        "test-wf",
			ProjectID: "proj",
			ScopeType: "ticket",
			Phases:    `[{"agent":"analyzer","layer":0}]`,
		}
		wf.SetGroups([]string{"be", "fe"})

		agentDef := &model.AgentDefinition{
			ID:         "analyzer",
			ProjectID:  "proj",
			WorkflowID: "test-wf",
			Model:      "sonnet",
			Timeout:    20,
			Prompt:     "p",
			Tag:        "be",
		}

		workflows, agents := service.BuildSpawnerConfig([]*model.Workflow{wf}, []*model.AgentDefinition{agentDef})

		wfDef, ok := workflows["test-wf"]
		if !ok {
			t.Fatalf("workflow test-wf not in spawner config")
		}
		if len(wfDef.Groups) != 2 || wfDef.Groups[0] != "be" || wfDef.Groups[1] != "fe" {
			t.Errorf("Groups = %v, want [be fe]", wfDef.Groups)
		}

		agentCfg, ok := agents["analyzer"]
		if !ok {
			t.Fatalf("agent analyzer not in spawner config")
		}
		if agentCfg.Tag != "be" {
			t.Errorf("Tag = %q, want %q", agentCfg.Tag, "be")
		}
	})

	t.Run("Empty Groups and Tag default correctly", func(t *testing.T) {
		wf := &model.Workflow{
			ID:        "wf-empty",
			ProjectID: "proj",
			ScopeType: "ticket",
			Phases:    `[{"agent":"builder","layer":0}]`,
			Groups:    "[]",
		}
		agentDef := &model.AgentDefinition{
			ID:         "builder",
			ProjectID:  "proj",
			WorkflowID: "wf-empty",
			Model:      "haiku",
			Timeout:    10,
			Prompt:     "p",
			Tag:        "",
		}

		workflows, agents := service.BuildSpawnerConfig([]*model.Workflow{wf}, []*model.AgentDefinition{agentDef})

		if g := workflows["wf-empty"].Groups; len(g) != 0 {
			t.Errorf("Groups = %v, want []", g)
		}
		if tag := agents["builder"].Tag; tag != "" {
			t.Errorf("Tag = %q, want empty", tag)
		}
	})
}
