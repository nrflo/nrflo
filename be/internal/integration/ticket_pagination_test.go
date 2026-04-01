package integration

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// insertPaginationTickets seeds a project and 4 tickets with distinct priorities and updated_at.
// Default sort order (updated_at DESC): PG-A, PG-B, PG-C, PG-D.
// Priority ASC order: PG-B(1), PG-D(2), PG-A(3), PG-C(4).
// Priority DESC order: PG-C(4), PG-A(3), PG-D(2), PG-B(1).
func insertPaginationTickets(t *testing.T, database *db.DB, projectID string, now time.Time) {
	t.Helper()
	nowStr := now.Format(time.RFC3339Nano)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID, nowStr, nowStr,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	tickets := []struct {
		id        string
		priority  int
		updatedAt time.Time
	}{
		{"PG-A", 3, now.Add(-1 * time.Hour)},
		{"PG-B", 1, now.Add(-2 * time.Hour)},
		{"PG-C", 4, now.Add(-3 * time.Hour)},
		{"PG-D", 2, now.Add(-4 * time.Hour)},
	}
	createdAt := now.Add(-5 * time.Hour).Format(time.RFC3339Nano)
	for _, tc := range tickets {
		if _, err := database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', 'open', ?, 'task', ?, ?, 'tester')`,
			tc.id, projectID, tc.id, tc.priority, createdAt,
			tc.updatedAt.Format(time.RFC3339Nano),
		); err != nil {
			t.Fatalf("insert ticket %s: %v", tc.id, err)
		}
	}
}

// openPaginationDB creates an isolated test DB with 4 test tickets.
func openPaginationDB(t *testing.T, projectID string) *repo.TicketRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	insertPaginationTickets(t, database, projectID, time.Now().UTC())
	return repo.NewTicketRepo(database, clock.Real())
}

// TestListWithBlockedInfo_Pagination verifies page/per_page behavior and metadata.
func TestListWithBlockedInfo_Pagination(t *testing.T) {
	r := openPaginationDB(t, "pgproj")

	cases := []struct {
		name        string
		page        int
		perPage     int
		wantTotal   int
		wantCount   int
		wantPage    int
		wantPerPage int
		wantFirstID string
	}{
		// Default sort (updated_at DESC): PG-A, PG-B, PG-C, PG-D
		{"first-page", 1, 2, 4, 2, 1, 2, "PG-A"},
		{"second-page", 2, 2, 4, 2, 2, 2, "PG-C"},
		{"beyond-total", 10, 2, 4, 0, 10, 2, ""},
		// Zero values → defaults: page=1, per_page=30
		{"zero-defaults", 0, 0, 4, 4, 1, 30, "PG-A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := r.ListWithBlockedInfo(&repo.ListFilter{
				ProjectID: "pgproj",
				Page:      tc.page,
				PerPage:   tc.perPage,
			})
			if err != nil {
				t.Fatalf("ListWithBlockedInfo: %v", err)
			}
			if result.TotalCount != tc.wantTotal {
				t.Errorf("TotalCount = %d, want %d", result.TotalCount, tc.wantTotal)
			}
			if len(result.Tickets) != tc.wantCount {
				t.Fatalf("len(Tickets) = %d, want %d", len(result.Tickets), tc.wantCount)
			}
			if result.Page != tc.wantPage {
				t.Errorf("Page = %d, want %d", result.Page, tc.wantPage)
			}
			if result.PerPage != tc.wantPerPage {
				t.Errorf("PerPage = %d, want %d", result.PerPage, tc.wantPerPage)
			}
			if tc.wantFirstID != "" && result.Tickets[0].ID != tc.wantFirstID {
				t.Errorf("Tickets[0].ID = %q, want %q", result.Tickets[0].ID, tc.wantFirstID)
			}
		})
	}
}

// TestListWithBlockedInfo_SortByPriority verifies sort_by=priority&sort_order=asc.
func TestListWithBlockedInfo_SortByPriority(t *testing.T) {
	r := openPaginationDB(t, "sortproj")

	result, err := r.ListWithBlockedInfo(&repo.ListFilter{
		ProjectID: "sortproj",
		SortBy:    "priority",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListWithBlockedInfo: %v", err)
	}
	if result.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", result.TotalCount)
	}
	if len(result.Tickets) != 4 {
		t.Fatalf("len(Tickets) = %d, want 4", len(result.Tickets))
	}
	// Priority ASC: PG-B(1), PG-D(2), PG-A(3), PG-C(4)
	wantOrder := []string{"PG-B", "PG-D", "PG-A", "PG-C"}
	for i, want := range wantOrder {
		if result.Tickets[i].ID != want {
			t.Errorf("pos %d: got %q, want %q", i, result.Tickets[i].ID, want)
		}
	}
}

// TestListWithBlockedInfo_InvalidSort verifies fallback behavior for bad sort inputs.
func TestListWithBlockedInfo_InvalidSort(t *testing.T) {
	r := openPaginationDB(t, "invproj")
	// Default sort (updated_at DESC): PG-A, PG-B, PG-C, PG-D
	defaultOrder := []string{"PG-A", "PG-B", "PG-C", "PG-D"}

	cases := []struct {
		name      string
		sortBy    string
		sortOrder string
		wantOrder []string
	}{
		{
			// Unknown column falls back to updated_at DESC; sort_order is ignored.
			"invalid-sort-by", "not_a_column", "asc", defaultOrder,
		},
		{
			// Empty sort_by falls back to default.
			"empty-sort-by", "", "asc", defaultOrder,
		},
		{
			// Valid column, invalid sort_order → treated as DESC.
			// Priority DESC: PG-C(4), PG-A(3), PG-D(2), PG-B(1)
			"invalid-sort-order", "priority", "descending",
			[]string{"PG-C", "PG-A", "PG-D", "PG-B"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := r.ListWithBlockedInfo(&repo.ListFilter{
				ProjectID: "invproj",
				SortBy:    tc.sortBy,
				SortOrder: tc.sortOrder,
			})
			if err != nil {
				t.Fatalf("ListWithBlockedInfo: %v", err)
			}
			if len(result.Tickets) != 4 {
				t.Fatalf("len(Tickets) = %d, want 4", len(result.Tickets))
			}
			for i, want := range tc.wantOrder {
				if result.Tickets[i].ID != want {
					t.Errorf("pos %d: got %q, want %q", i, result.Tickets[i].ID, want)
				}
			}
		})
	}
}

// TestListWithBlockedInfo_BlockedOnlyPagination verifies COUNT and LIMIT work with the
// BlockedOnly subquery WHERE clause.
func TestListWithBlockedInfo_BlockedOnlyPagination(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	insertPaginationTickets(t, database, "blkproj", now)

	// PG-B and PG-C are blocked by PG-A (which is open).
	nowStr := now.Format(time.RFC3339Nano)
	for _, blocked := range []string{"PG-B", "PG-C"} {
		if _, err := database.Exec(`
			INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
			VALUES (?, ?, 'PG-A', 'blocks', ?, 'tester')`, "blkproj", blocked, nowStr); err != nil {
			t.Fatalf("insert dep for %s: %v", blocked, err)
		}
	}

	r := repo.NewTicketRepo(database, clock.Real())

	// All blocked tickets: TotalCount=2, per_page=10 → all returned on page 1.
	result, err := r.ListWithBlockedInfo(&repo.ListFilter{
		ProjectID:   "blkproj",
		BlockedOnly: true,
		Page:        1,
		PerPage:     10,
	})
	if err != nil {
		t.Fatalf("ListWithBlockedInfo: %v", err)
	}
	if result.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", result.TotalCount)
	}
	if len(result.Tickets) != 2 {
		t.Fatalf("len(Tickets) = %d, want 2", len(result.Tickets))
	}
	for _, pt := range result.Tickets {
		if !pt.IsBlocked {
			t.Errorf("ticket %s: IsBlocked = false, want true", pt.ID)
		}
	}

	// Pagination with BlockedOnly: page=1,per_page=1 → TotalCount still 2, one ticket.
	result, err = r.ListWithBlockedInfo(&repo.ListFilter{
		ProjectID:   "blkproj",
		BlockedOnly: true,
		Page:        1,
		PerPage:     1,
	})
	if err != nil {
		t.Fatalf("ListWithBlockedInfo per_page=1: %v", err)
	}
	if result.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2 (COUNT must use BlockedOnly subquery)", result.TotalCount)
	}
	if len(result.Tickets) != 1 {
		t.Errorf("len(Tickets) = %d, want 1", len(result.Tickets))
	}
}
