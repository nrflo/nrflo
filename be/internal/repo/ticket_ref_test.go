package repo

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedRefProject inserts a minimal project row for ticket_ref tests.
func seedRefProject(t *testing.T, q db.Querier, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := q.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seedRefProject(%q): %v", projectID, err)
	}
}

// seedRefTicket inserts a minimal ticket row for ticket_ref tests.
func seedRefTicket(t *testing.T, q db.Querier, projectID, ticketID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := q.Exec(
		`INSERT OR IGNORE INTO tickets (id, project_id, title, status, priority, issue_type, created_at, updated_at, created_by)
		 VALUES (?, ?, 'Test ticket', 'open', 2, 'task', ?, ?, 'test')`,
		ticketID, projectID, now, now,
	); err != nil {
		t.Fatalf("seedRefTicket(%q/%q): %v", projectID, ticketID, err)
	}
}

func TestTicketRefRepo_Create_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC))
	r := NewTicketRefRepo(pool, clk)

	seedRefProject(t, pool, "proj-create")
	seedRefTicket(t, pool, "proj-create", "tkt-1")

	ref := &model.TicketRef{
		ProjectID: "PROJ-CREATE",
		TicketID:  "TKT-1",
		Kind:      string(model.KindPR),
		URL:       "https://github.com/org/repo/pull/42",
		Label:     sql.NullString{String: "PR #42", Valid: true},
	}

	if err := r.Create(ref); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if ref.ID == 0 {
		t.Errorf("ID not set after Create, want > 0")
	}
	if ref.ProjectID != "proj-create" {
		t.Errorf("ProjectID = %q, want %q", ref.ProjectID, "proj-create")
	}
	if ref.TicketID != "tkt-1" {
		t.Errorf("TicketID = %q, want %q", ref.TicketID, "tkt-1")
	}
	if ref.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not set after Create")
	}

	refs, err := r.ListByTicket("proj-create", "tkt-1")
	if err != nil {
		t.Fatalf("ListByTicket: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("len(refs) = %d, want 1", len(refs))
	}
	got := refs[0]
	if got.ID != ref.ID {
		t.Errorf("ID = %d, want %d", got.ID, ref.ID)
	}
	if got.Kind != string(model.KindPR) {
		t.Errorf("Kind = %q, want %q", got.Kind, model.KindPR)
	}
	if got.URL != ref.URL {
		t.Errorf("URL = %q, want %q", got.URL, ref.URL)
	}
	if !got.Label.Valid || got.Label.String != "PR #42" {
		t.Errorf("Label = %v, want {String:'PR #42', Valid:true}", got.Label)
	}
}

func TestTicketRefRepo_BulkCreate_ThreeRefs(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 2, 1, 8, 0, 0, 0, time.UTC))
	r := NewTicketRefRepo(pool, clk)

	seedRefProject(t, pool, "proj-bulk")
	seedRefTicket(t, pool, "proj-bulk", "tkt-bulk")

	refs := []*model.TicketRef{
		{ProjectID: "proj-bulk", TicketID: "tkt-bulk", Kind: string(model.KindSource), URL: "https://example.com/source"},
		{ProjectID: "proj-bulk", TicketID: "tkt-bulk", Kind: string(model.KindRelated), URL: "https://example.com/related"},
		{ProjectID: "proj-bulk", TicketID: "tkt-bulk", Kind: string(model.KindDesignDoc), URL: "https://example.com/design"},
	}

	if err := r.BulkCreate(refs); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}

	for i, ref := range refs {
		if ref.ID == 0 {
			t.Errorf("refs[%d].ID not set after BulkCreate", i)
		}
	}

	got, err := r.ListByTicket("proj-bulk", "tkt-bulk")
	if err != nil {
		t.Fatalf("ListByTicket: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}

	wantKinds := []string{string(model.KindSource), string(model.KindRelated), string(model.KindDesignDoc)}
	for i, g := range got {
		if g.Kind != wantKinds[i] {
			t.Errorf("got[%d].Kind = %q, want %q", i, g.Kind, wantKinds[i])
		}
	}
}

func TestTicketRefRepo_BulkCreate_Empty_NoOp(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC))
	r := NewTicketRefRepo(pool, clk)

	if err := r.BulkCreate(nil); err != nil {
		t.Errorf("BulkCreate(nil): %v, want nil", err)
	}
	if err := r.BulkCreate([]*model.TicketRef{}); err != nil {
		t.Errorf("BulkCreate([]): %v, want nil", err)
	}
}

func TestTicketRefRepo_CascadeOnTicketDelete(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC))
	r := NewTicketRefRepo(pool, clk)

	seedRefProject(t, pool, "proj-cascade")
	seedRefTicket(t, pool, "proj-cascade", "tkt-cascade")

	refs := []*model.TicketRef{
		{ProjectID: "proj-cascade", TicketID: "tkt-cascade", Kind: string(model.KindPR), URL: "https://example.com/pr/1"},
		{ProjectID: "proj-cascade", TicketID: "tkt-cascade", Kind: string(model.KindSource), URL: "https://example.com/src"},
	}
	if err := r.BulkCreate(refs); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}

	before, err := r.ListByTicket("proj-cascade", "tkt-cascade")
	if err != nil {
		t.Fatalf("ListByTicket before delete: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("expected 2 refs before delete, got %d", len(before))
	}

	if _, err := pool.Exec(
		`DELETE FROM tickets WHERE project_id = ? AND id = ?`,
		"proj-cascade", "tkt-cascade",
	); err != nil {
		t.Fatalf("delete ticket: %v", err)
	}

	after, err := r.ListByTicket("proj-cascade", "tkt-cascade")
	if err != nil {
		t.Fatalf("ListByTicket after delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("len(after) = %d, want 0 (ON DELETE CASCADE should remove ticket_refs)", len(after))
	}
}

func TestTicketRefRepo_ListByTicket_CaseInsensitive(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC))
	r := NewTicketRefRepo(pool, clk)

	seedRefProject(t, pool, "proj-ci")
	seedRefTicket(t, pool, "proj-ci", "tkt-ci")

	ref := &model.TicketRef{
		ProjectID: "proj-ci",
		TicketID:  "tkt-ci",
		Kind:      string(model.KindRelated),
		URL:       "https://example.com/related",
	}
	if err := r.Create(ref); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.ListByTicket("PROJ-CI", "TKT-CI")
	if err != nil {
		t.Fatalf("ListByTicket(uppercase): %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (case-insensitive lookup)", len(got))
	}
}

func TestTicketRefKind_IsValidKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind  string
		valid bool
	}{
		{string(model.KindSource), true},
		{string(model.KindRelated), true},
		{string(model.KindPR), true},
		{string(model.KindDesignDoc), true},
		{"unknown", false},
		{"", false},
		{"PR", false},
	}
	for _, c := range cases {
		if got := model.IsValidKind(c.kind); got != c.valid {
			t.Errorf("IsValidKind(%q) = %v, want %v", c.kind, got, c.valid)
		}
	}
}
