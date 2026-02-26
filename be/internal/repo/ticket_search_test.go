package repo

import (
	"database/sql"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// --- sanitizeFTS5Query unit tests ---

func TestSanitizeFTS5Query(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ticket ID with hyphen",
			input: "REF-12495",
			want:  `"REF" "12495"`,
		},
		{
			name:  "bare number",
			input: "12495",
			want:  `"12495"`,
		},
		{
			name:  "FTS5 operators",
			input: "AND OR NOT",
			want:  `"AND" "OR" "NOT"`,
		},
		{
			name:  "only special chars",
			input: "---",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "plain text",
			input: "login bug",
			want:  `"login" "bug"`,
		},
		{
			// double-quote is a non-alphanumeric separator, so tokens never
			// contain embedded quotes; the ReplaceAll for `"` is a safety net.
			name:  "double quote as separator",
			input: `say "hello"`,
			want:  `"say" "hello"`,
		},
		{
			name:  "colon separated",
			input: "project:foo",
			want:  `"project" "foo"`,
		},
		{
			name:  "asterisk glob",
			input: "foo*",
			want:  `"foo"`,
		},
		{
			name:  "multiple hyphens",
			input: "abc-def-ghi",
			want:  `"abc" "def" "ghi"`,
		},
		{
			name:  "leading trailing special chars",
			input: "---hello---",
			want:  `"hello"`,
		},
		{
			name:  "single word",
			input: "hello",
			want:  `"hello"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeFTS5Query(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeFTS5Query(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- integration helpers ---

func newSearchTestDB(t *testing.T) (*TicketRepo, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}
	repo := NewTicketRepo(database, clock.Real())
	return repo, func() { database.Close() }
}

func mustCreateTicket(t *testing.T, repo *TicketRepo, ticket *model.Ticket) {
	t.Helper()
	if err := repo.Create(ticket); err != nil {
		t.Fatalf("failed to create ticket %q: %v", ticket.ID, err)
	}
}

// --- TicketRepo.Search integration tests ---

func TestSearchTicketIDStyle(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "REF-12495",
		ProjectID: "test-proj",
		Title:     "Reference ticket",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	})

	// Should not error; should return the ticket
	tickets, err := repo.Search("test-proj", "REF-12495")
	if err != nil {
		t.Fatalf("Search(\"REF-12495\") returned error: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("Search(\"REF-12495\") returned %d tickets, want 1", len(tickets))
	}
	if tickets[0].ID != "ref-12495" {
		t.Errorf("Search returned ticket ID %q, want %q", tickets[0].ID, "ref-12495")
	}
}

func TestSearchBareNumber(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "TASK-12495",
		ProjectID: "test-proj",
		Title:     "Task 12495",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	})

	// Bare number should not cause SQL error
	tickets, err := repo.Search("test-proj", "12495")
	if err != nil {
		t.Fatalf("Search(\"12495\") returned error: %v", err)
	}
	// Should match ticket whose ID contains "12495"
	if len(tickets) != 1 {
		t.Fatalf("Search(\"12495\") returned %d tickets, want 1", len(tickets))
	}
}

func TestSearchFTS5OperatorWords(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	for _, word := range []string{"AND", "OR", "NOT"} {
		word := word
		t.Run(word, func(t *testing.T) {
			_, err := repo.Search("test-proj", word)
			if err != nil {
				t.Errorf("Search(%q) returned unexpected error: %v", word, err)
			}
		})
	}
}

func TestSearchOnlySpecialChars(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	// "---" sanitizes to empty → should return nil, not error
	tickets, err := repo.Search("test-proj", "---")
	if err != nil {
		t.Fatalf("Search(\"---\") returned error: %v", err)
	}
	if tickets != nil {
		t.Errorf("Search(\"---\") returned non-nil tickets, want nil; got %v", tickets)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	tickets, err := repo.Search("test-proj", "")
	if err != nil {
		t.Fatalf("Search(\"\") returned error: %v", err)
	}
	if tickets != nil {
		t.Errorf("Search(\"\") returned non-nil tickets, want nil")
	}
}

func TestSearchDescriptionContent(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:          "FEAT-001",
		ProjectID:   "test-proj",
		Title:       "Feature ticket",
		Description: sql.NullString{String: "Implements the authentication flow for the login page", Valid: true},
		Status:      model.StatusOpen,
		Priority:    2,
		IssueType:   model.IssueTypeTask,
		CreatedBy:   "tester",
	})

	tickets, err := repo.Search("test-proj", "authentication")
	if err != nil {
		t.Fatalf("Search(\"authentication\") returned error: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("Search(\"authentication\") returned %d tickets, want 1", len(tickets))
	}
	if tickets[0].ID != "feat-001" {
		t.Errorf("got ticket ID %q, want %q", tickets[0].ID, "feat-001")
	}
}

func TestSearchPlainText(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "BUG-001",
		ProjectID: "test-proj",
		Title:     "Login bug fix",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeBug,
		CreatedBy: "tester",
	})
	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "TASK-001",
		ProjectID: "test-proj",
		Title:     "Unrelated task",
		Status:    model.StatusOpen,
		Priority:  3,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	})

	tickets, err := repo.Search("test-proj", "login bug")
	if err != nil {
		t.Fatalf("Search(\"login bug\") returned error: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("Search(\"login bug\") returned %d tickets, want 1", len(tickets))
	}
	if tickets[0].ID != "bug-001" {
		t.Errorf("got ticket ID %q, want %q", tickets[0].ID, "bug-001")
	}
}

func TestSearchProjectIsolation(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	// Insert a second project
	database, err := db.OpenPath(filepath.Join(t.TempDir(), "iso.db"))
	if err != nil {
		t.Fatalf("open second db: %v", err)
	}
	defer database.Close()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "ALPHA-1",
		ProjectID: "test-proj",
		Title:     "Alpha ticket",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	})

	// Search in a different project — should not return results from test-proj
	tickets, err := repo.Search("other-proj", "Alpha")
	if err != nil {
		t.Fatalf("Search on other-proj returned error: %v", err)
	}
	if len(tickets) != 0 {
		t.Errorf("Search on other-proj returned %d tickets, want 0", len(tickets))
	}
}

// --- SearchWithBlockedInfo integration tests ---

func TestSearchWithBlockedInfoTicketID(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	mustCreateTicket(t, repo, &model.Ticket{
		ID:        "REF-99",
		ProjectID: "test-proj",
		Title:     "Ref ticket ninety-nine",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	})

	results, err := repo.SearchWithBlockedInfo("test-proj", "REF-99")
	if err != nil {
		t.Fatalf("SearchWithBlockedInfo(\"REF-99\") returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchWithBlockedInfo(\"REF-99\") returned %d results, want 1", len(results))
	}
	if results[0].ID != "ref-99" {
		t.Errorf("got ticket ID %q, want %q", results[0].ID, "ref-99")
	}
	if results[0].IsBlocked {
		t.Errorf("expected ticket not blocked")
	}
}

func TestSearchWithBlockedInfoEmptySanitized(t *testing.T) {
	repo, cleanup := newSearchTestDB(t)
	defer cleanup()

	results, err := repo.SearchWithBlockedInfo("test-proj", "---")
	if err != nil {
		t.Fatalf("SearchWithBlockedInfo(\"---\") returned error: %v", err)
	}
	// attachBlockedInfo returns an empty non-nil slice when given nil tickets
	if len(results) != 0 {
		t.Errorf("SearchWithBlockedInfo(\"---\") returned %d results, want 0", len(results))
	}
}
