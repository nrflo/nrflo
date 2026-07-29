package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func TestConsult_CreateSetChildSessionRoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewConsultRepo(pool, clock.Real())

	c := &model.Consult{
		ID:                 "consult.abcd1234",
		CallerSessionID:    "caller-sess",
		WorkflowInstanceID: "wfi-1",
		ProjectID:          "proj-1",
		ConsultantID:       "security-consultant",
		Question:           "is this safe?",
	}
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := r.ListByInstance("wfi-1")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	got := list[0]
	if got.CallerSessionID != c.CallerSessionID || got.ProjectID != c.ProjectID ||
		got.ConsultantID != c.ConsultantID || got.Question != c.Question {
		t.Errorf("got = %+v, want fields matching seed %+v", got, c)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.ChildSessionID != "" {
		t.Errorf("ChildSessionID = %q, want empty on a fresh row", got.ChildSessionID)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil on a fresh row", got.CompletedAt)
	}

	if err := r.SetChildSession(c.ID, "child-sess-1"); err != nil {
		t.Fatalf("SetChildSession: %v", err)
	}
	list, err = r.ListByInstance("wfi-1")
	if err != nil {
		t.Fatalf("ListByInstance after SetChildSession: %v", err)
	}
	if list[0].ChildSessionID != "child-sess-1" {
		t.Errorf("ChildSessionID = %q, want child-sess-1", list[0].ChildSessionID)
	}
}

func TestConsult_MarkTerminal_SetsStatusAndGuardsSecondCall(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewConsultRepo(pool, clock.Real())

	c := &model.Consult{ID: "consult.term01", CallerSessionID: "c", WorkflowInstanceID: "wfi-2", ProjectID: "proj-1", ConsultantID: "cons"}
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	won, err := r.MarkTerminal(c.ID, "completed", "")
	if err != nil {
		t.Fatalf("MarkTerminal: %v", err)
	}
	if !won {
		t.Fatal("MarkTerminal first call: won = false, want true")
	}

	list, err := r.ListByInstance("wfi-2")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if list[0].Status != "completed" {
		t.Errorf("Status = %q, want completed", list[0].Status)
	}
	if list[0].CompletedAt == nil {
		t.Error("CompletedAt = nil, want set")
	}

	// Second call loses the status='running' guard: it must report it did not
	// win, and must not silently rewrite the row's status/error.
	won2, err := r.MarkTerminal(c.ID, "failed", "boom")
	if err != nil {
		t.Fatalf("MarkTerminal (second call): %v", err)
	}
	if won2 {
		t.Error("MarkTerminal second call: won = true, want false (already terminal)")
	}
	list2, err := r.ListByInstance("wfi-2")
	if err != nil {
		t.Fatalf("ListByInstance after second MarkTerminal: %v", err)
	}
	if list2[0].Status != "completed" || list2[0].Error != "" {
		t.Errorf("row after guarded second call = %+v, want unchanged completed/no error", list2[0])
	}
}

func TestConsult_MarkTerminal_Failed_RecordsError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewConsultRepo(pool, clock.Real())

	c := &model.Consult{ID: "consult.err01", CallerSessionID: "c", WorkflowInstanceID: "wfi-3", ProjectID: "proj-1", ConsultantID: "cons"}
	if err := r.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := r.MarkTerminal(c.ID, "failed", "spawn failed: no api key"); err != nil {
		t.Fatalf("MarkTerminal: %v", err)
	}

	list, err := r.ListByInstance("wfi-3")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if list[0].Status != "failed" || list[0].Error != "spawn failed: no api key" {
		t.Errorf("got = %+v, want failed/spawn failed: no api key", list[0])
	}
}

func TestConsult_ListByInstance_OrderedByCreatedAtAscAndScopedToInstance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewConsultRepo(pool, clock.Real())

	c1 := &model.Consult{ID: "consult.a", CallerSessionID: "c", WorkflowInstanceID: "wfi-4", ProjectID: "proj-1", ConsultantID: "cons"}
	c2 := &model.Consult{ID: "consult.b", CallerSessionID: "c", WorkflowInstanceID: "wfi-4", ProjectID: "proj-1", ConsultantID: "cons"}
	other := &model.Consult{ID: "consult.other", CallerSessionID: "c", WorkflowInstanceID: "wfi-5", ProjectID: "proj-1", ConsultantID: "cons"}
	if err := r.Create(c1); err != nil {
		t.Fatalf("Create c1: %v", err)
	}
	if err := r.Create(c2); err != nil {
		t.Fatalf("Create c2: %v", err)
	}
	if err := r.Create(other); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	list, err := r.ListByInstance("wfi-4")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2 (scoped to wfi-4)", len(list))
	}
	if list[0].ID != "consult.a" || list[1].ID != "consult.b" {
		t.Errorf("order = %s,%s, want consult.a,consult.b (created_at ASC)", list[0].ID, list[1].ID)
	}
}

func TestConsult_ListByInstance_UnknownInstance_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewConsultRepo(pool, clock.Real())

	list, err := r.ListByInstance("no-such-wfi")
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}
