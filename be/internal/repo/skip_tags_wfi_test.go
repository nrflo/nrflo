// Package repo tests for skip-tags: agent definition tag and workflow instance skip_tags.
package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// --- Agent Definition Tag ---

func TestAgentDefinitionTagCreateAndGet(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	agentRepo := NewAgentDefinitionRepo(pool, clock.Real())

	cases := []struct {
		defID string
		tag   string
	}{
		{"agent-tag-0", ""},
		{"agent-tag-1", "be"},
		{"agent-tag-2", "frontend"},
	}

	for _, tc := range cases {
		t.Run("tag="+tc.tag, func(t *testing.T) {
			def := &model.AgentDefinition{
				ID: tc.defID, ProjectID: "proj", WorkflowID: "wf",
				Model: "sonnet", Timeout: 20, Prompt: "prompt", Tag: tc.tag,
			}
			if err := agentRepo.Create(def); err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := agentRepo.Get("proj", "wf", def.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Tag != tc.tag {
				t.Errorf("Tag = %q, want %q", got.Tag, tc.tag)
			}
		})
	}
}

func TestAgentDefinitionTagUpdate(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	agentRepo := NewAgentDefinitionRepo(pool, clock.Real())

	def := &model.AgentDefinition{
		ID: "agent-upd-tag", ProjectID: "proj", WorkflowID: "wf",
		Model: "sonnet", Timeout: 20, Prompt: "prompt", Tag: "",
	}
	if err := agentRepo.Create(def); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newTag := "be"
	if err := agentRepo.Update("proj", "wf", "agent-upd-tag", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update tag: %v", err)
	}
	got, err := agentRepo.Get("proj", "wf", "agent-upd-tag")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Tag != "be" {
		t.Errorf("Tag after update = %q, want %q", got.Tag, "be")
	}

	emptyTag := ""
	agentRepo.Update("proj", "wf", "agent-upd-tag", &AgentDefUpdateFields{Tag: &emptyTag})
	got2, _ := agentRepo.Get("proj", "wf", "agent-upd-tag")
	if got2.Tag != "" {
		t.Errorf("Tag after empty update = %q, want empty", got2.Tag)
	}
}

func TestAgentDefinitionTagInList(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	agentRepo := NewAgentDefinitionRepo(pool, clock.Real())

	for _, d := range []*model.AgentDefinition{
		{ID: "ag-list-1", ProjectID: "proj", WorkflowID: "wf", Model: "sonnet", Timeout: 20, Prompt: "p", Tag: "be"},
		{ID: "ag-list-2", ProjectID: "proj", WorkflowID: "wf", Model: "haiku", Timeout: 10, Prompt: "p", Tag: ""},
	} {
		if err := agentRepo.Create(d); err != nil {
			t.Fatalf("Create %s: %v", d.ID, err)
		}
	}

	list, err := agentRepo.List("proj", "wf")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}

	byID := make(map[string]*model.AgentDefinition)
	for _, d := range list {
		byID[d.ID] = d
	}
	if byID["ag-list-1"].Tag != "be" {
		t.Errorf("ag-list-1 Tag = %q, want be", byID["ag-list-1"].Tag)
	}
	if byID["ag-list-2"].Tag != "" {
		t.Errorf("ag-list-2 Tag = %q, want empty", byID["ag-list-2"].Tag)
	}
}

func TestAgentDefinitionTagDefaultEmpty(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")

	_, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES ('ag-no-tag', 'proj', 'wf', 'sonnet', 20, 'p', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := NewAgentDefinitionRepo(pool, clock.Real()).Get("proj", "wf", "ag-no-tag")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Tag != "" {
		t.Errorf("Tag default = %q, want empty", got.Tag)
	}
}

// --- WorkflowInstance SkipTags ---

func TestWorkflowInstanceSkipTagsCreateAndGet(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())

	wi := &model.WorkflowInstance{
		ID: "wfi-st-1", ProjectID: "proj", TicketID: "ticket-1",
		WorkflowID: "wf", ScopeType: "ticket", Status: model.WorkflowInstanceActive,
		Findings: "{}", SkipTags: "[]",
	}
	wi.SetSkipTags([]string{"be", "fe"})

	if err := wfiRepo.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := wfiRepo.Get("wfi-st-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tags := got.GetSkipTags(); len(tags) != 2 || tags[0] != "be" || tags[1] != "fe" {
		t.Errorf("GetSkipTags() = %v, want [be fe]", tags)
	}
}

func TestWorkflowInstanceSkipTagsDefaultEmpty(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())

	_, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-no-st', 'proj', 'ticket-2', 'wf', 'ticket', 'active', '{}', 0, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := wfiRepo.Get("wfi-no-st")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tags := got.GetSkipTags(); len(tags) != 0 {
		t.Errorf("default skip_tags = %v, want empty", tags)
	}
}

func TestWorkflowInstanceUpdateSkipTags(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())

	wi := &model.WorkflowInstance{
		ID: "wfi-upd-st", ProjectID: "proj", TicketID: "ticket-upd",
		WorkflowID: "wf", ScopeType: "ticket", Status: model.WorkflowInstanceActive,
		Findings: "{}", SkipTags: "[]",
	}
	if err := wfiRepo.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cases := []struct {
		skipTags string
		want     []string
	}{
		{`["be"]`, []string{"be"}},
		{`["be","fe"]`, []string{"be", "fe"}},
		{`[]`, []string{}},
	}

	for _, tc := range cases {
		if err := wfiRepo.UpdateSkipTags("wfi-upd-st", tc.skipTags); err != nil {
			t.Fatalf("UpdateSkipTags(%s): %v", tc.skipTags, err)
		}
		got, err := wfiRepo.Get("wfi-upd-st")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if tags := got.GetSkipTags(); len(tags) != len(tc.want) {
			t.Errorf("GetSkipTags() = %v, want %v", tags, tc.want)
		}
	}
}

func TestWorkflowInstanceUpdateSkipTagsNotFound(t *testing.T) {
	pool := newSkipTagsPool(t)
	seedProjectPool(t, pool, "proj")
	seedWorkflowPool(t, pool, "proj", "wf")
	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())

	if err := wfiRepo.UpdateSkipTags("nonexistent-id", `["be"]`); err == nil {
		t.Error("expected error for nonexistent workflow instance, got nil")
	}
}

// --- WorkflowInstance model unit tests ---

func TestWorkflowInstanceGetSetSkipTagsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		tags []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"one", []string{"be"}},
		{"two", []string{"be", "fe"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wi := &model.WorkflowInstance{}
			wi.SetSkipTags(tc.tags)
			got := wi.GetSkipTags()
			want := tc.tags
			if want == nil {
				want = []string{}
			}
			if len(got) != len(want) {
				t.Errorf("GetSkipTags() = %v, want %v", got, want)
			}
		})
	}
}

func TestWorkflowInstanceAddSkipTagDeduplication(t *testing.T) {
	wi := &model.WorkflowInstance{SkipTags: "[]"}
	wi.AddSkipTag("be")
	wi.AddSkipTag("be") // duplicate
	wi.AddSkipTag("fe")
	wi.AddSkipTag("be") // duplicate again

	tags := wi.GetSkipTags()
	if len(tags) != 2 || tags[0] != "be" || tags[1] != "fe" {
		t.Errorf("AddSkipTag dedup: %v, want [be fe]", tags)
	}
}

func TestWorkflowInstanceAddSkipTagFromEmpty(t *testing.T) {
	wi := &model.WorkflowInstance{}
	wi.AddSkipTag("docs")
	if tags := wi.GetSkipTags(); len(tags) != 1 || tags[0] != "docs" {
		t.Errorf("AddSkipTag on empty: %v, want [docs]", tags)
	}
}
