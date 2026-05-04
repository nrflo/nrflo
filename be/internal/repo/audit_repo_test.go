package repo

import (
	"fmt"
	"testing"
	"time"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/model"

	"github.com/google/uuid"
)

const seedAdminID = "usr_admin_seed" // always present in template DB via migration 000078

// insertAuditUser inserts a minimal user row via raw SQL so audit entries can pass FK.
func insertAuditUser(t *testing.T, r *AuditRepo, id, email string) {
	t.Helper()
	hash, err := auth.Hash("auditTestPass")
	if err != nil {
		t.Fatalf("hash for audit user: %v", err)
	}
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err = r.db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash, role, status, must_change_password, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, email, email, hash, "viewer", "active", 0, now, now,
	)
	if err != nil {
		t.Fatalf("insert audit user %q: %v", id, err)
	}
}

func setupAuditRepoWithClock(t *testing.T, clk clock.Clock) *AuditRepo {
	t.Helper()
	db := newTestDB(t)
	return NewAuditRepo(db, clk)
}

func TestAuditRepo_Append_SetsCreatedAtFromClock(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2025, 4, 1, 9, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	r := setupAuditRepoWithClock(t, clk)

	e := &model.AuditEntry{
		ID: "ae-clock-1", UserID: seedAdminID, Action: "login_success",
	}
	if err := r.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, _, err := r.List(model.AuditFilter{Action: "login_success"}, 1, 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got *model.AuditEntry
	for _, en := range entries {
		if en.ID == "ae-clock-1" {
			got = en
		}
	}
	if got == nil {
		t.Fatal("entry ae-clock-1 not found")
	}
	if !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, fixedTime)
	}
}

func TestAuditRepo_Append_ExplicitCreatedAt_NotOverridden(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	r := setupAuditRepoWithClock(t, clk)

	explicitTime := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	e := &model.AuditEntry{
		ID: "ae-explicit", UserID: seedAdminID, Action: "explicit_action",
		CreatedAt: explicitTime,
	}
	if err := r.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, _, err := r.List(model.AuditFilter{Action: "explicit_action"}, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("entry ae-explicit not found")
	}
	if !entries[0].CreatedAt.Equal(explicitTime) {
		t.Errorf("CreatedAt = %v, want %v (explicit time should not be overridden)", entries[0].CreatedAt, explicitTime)
	}
}

func TestAuditRepo_Append_DefaultMetadata(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	r := setupAuditRepoWithClock(t, clk)

	e := &model.AuditEntry{
		ID: "ae-meta", UserID: seedAdminID, Action: "meta_test_action",
	}
	if err := r.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, _, err := r.List(model.AuditFilter{Action: "meta_test_action"}, 1, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries returned")
	}
	if entries[0].Metadata != "{}" {
		t.Errorf("Metadata = %q, want {} (default)", entries[0].Metadata)
	}
}

func TestAuditRepo_List_NoFilter_DescOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(base)
	r := setupAuditRepoWithClock(t, clk)

	action := "desc_order_test_" + uuid.New().String()[:8]
	ids := []string{"ae-ord-a", "ae-ord-b", "ae-ord-c"}
	for i, id := range ids {
		clk.Set(base.Add(time.Duration(i) * time.Second))
		if err := r.Append(&model.AuditEntry{ID: id, UserID: seedAdminID, Action: action}); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	entries, total, err := r.List(model.AuditFilter{Action: action}, 1, 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(entries) != 3 {
		t.Fatalf("entries count = %d, want 3", len(entries))
	}
	// DESC order: newest first → ae-ord-c, ae-ord-b, ae-ord-a
	wantOrder := []string{"ae-ord-c", "ae-ord-b", "ae-ord-a"}
	for i, want := range wantOrder {
		if entries[i].ID != want {
			t.Errorf("entries[%d].ID = %q, want %q", i, entries[i].ID, want)
		}
	}
}

func TestAuditRepo_List_FilterByUserID(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(base)
	r := setupAuditRepoWithClock(t, clk)

	// Create a second user (seed admin is user A).
	insertAuditUser(t, r, "u-audit-b", "auditb@test.com")

	action := "user_filter_" + uuid.New().String()[:8]
	for i := 0; i < 3; i++ {
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("af-a-%d", i), UserID: seedAdminID, Action: action,
		}); err != nil {
			t.Fatalf("Append user A entry %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("af-b-%d", i), UserID: "u-audit-b", Action: action,
		}); err != nil {
			t.Fatalf("Append user B entry %d: %v", i, err)
		}
	}

	entries, total, err := r.List(model.AuditFilter{UserID: seedAdminID, Action: action}, 1, 100)
	if err != nil {
		t.Fatalf("List with UserID filter: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	for _, e := range entries {
		if e.UserID != seedAdminID {
			t.Errorf("entry %s has UserID %q, want %s", e.ID, e.UserID, seedAdminID)
		}
	}
}

func TestAuditRepo_List_FilterByAction(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	r := setupAuditRepoWithClock(t, clk)

	actions := []string{"action_x", "action_x", "action_y", "action_z"}
	for i, a := range actions {
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("faction-%d", i), UserID: seedAdminID, Action: a,
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	entries, total, err := r.List(model.AuditFilter{Action: "action_x"}, 1, 100)
	if err != nil {
		t.Fatalf("List action filter: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(entries) != 2 {
		t.Errorf("entries count = %d, want 2", len(entries))
	}
}

func TestAuditRepo_List_FilterByDateRange(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(base)
	r := setupAuditRepoWithClock(t, clk)

	action := "date_range_" + uuid.New().String()[:8]
	for i := 0; i < 5; i++ {
		clk.Set(base.Add(time.Duration(i) * time.Hour))
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("dr-%d", i), UserID: seedAdminID, Action: action,
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	since := base.Add(time.Hour)
	until := base.Add(3 * time.Hour)
	entries, total, err := r.List(model.AuditFilter{Action: action, Since: &since, Until: &until}, 1, 100)
	if err != nil {
		t.Fatalf("List with date range: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (hours 1,2,3)", total)
	}
	if len(entries) != 3 {
		t.Errorf("entries count = %d, want 3", len(entries))
	}
}

func TestAuditRepo_List_Pagination(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(base)
	r := setupAuditRepoWithClock(t, clk)

	action := "paging_test_" + uuid.New().String()[:8]
	for i := 0; i < 7; i++ {
		clk.Set(base.Add(time.Duration(i) * time.Second))
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("pag-%d", i), UserID: seedAdminID, Action: action,
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	page1, total, err := r.List(model.AuditFilter{Action: action}, 1, 3)
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	if len(page1) != 3 {
		t.Errorf("page1 count = %d, want 3", len(page1))
	}
	if page1[0].ID != "pag-6" {
		t.Errorf("page1[0].ID = %q, want pag-6 (most recent)", page1[0].ID)
	}

	page3, _, err := r.List(model.AuditFilter{Action: action}, 3, 3)
	if err != nil {
		t.Fatalf("List page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3 count = %d, want 1", len(page3))
	}
	if page3[0].ID != "pag-0" {
		t.Errorf("page3[0].ID = %q, want pag-0 (oldest)", page3[0].ID)
	}
}

func TestAuditRepo_List_Empty(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	r := setupAuditRepoWithClock(t, clk)

	entries, total, err := r.List(model.AuditFilter{Action: "nonexistent_action_" + uuid.New().String()}, 1, 100)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(entries) != 0 {
		t.Errorf("entries count = %d, want 0", len(entries))
	}
}
