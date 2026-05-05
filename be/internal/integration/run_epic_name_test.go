package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
)

// seedEpicWithChild inserts an epic ticket and a single open child into the DB.
func seedEpicWithChild(t *testing.T, database *db.DB, projectID, epicID, epicTitle, childID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'epic', 1, ?, ?, 'test')`,
		epicID, projectID, epicTitle, now, now)
	if err != nil {
		t.Fatalf("failed to insert epic %s: %v", epicID, err)
	}
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
		VALUES (?, ?, 'Child', 'open', 'feature', 2, ?, ?, ?, 'test')`,
		childID, projectID, epicID, now, now)
	if err != nil {
		t.Fatalf("failed to insert child %s: %v", childID, err)
	}
}

// runEpicRequest posts to /api/v1/tickets/:epicID/workflow/run-epic (start=false).
func runEpicRequest(t *testing.T, client *http.Client, baseURL, projectID, epicID string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"workflow_name": "test", "start": false})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+epicID+"/workflow/run-epic", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

// TestRunEpicWorkflow_EpicPrefixCases verifies that the chain name is not doubled
// when the ticket title already carries an "epic:" prefix (any casing), and that
// titles without such prefix correctly receive the "Epic: " prepend.
func TestRunEpicWorkflow_EpicPrefixCases(t *testing.T) {
	cases := []struct {
		name       string
		epicTitle  string
		wantName   string
	}{
		{
			name:      "title_without_prefix",
			epicTitle: "My Story",
			wantName:  "Epic: My Story",
		},
		{
			name:      "title_with_exact_prefix",
			epicTitle: "Epic: My Story",
			wantName:  "Epic: My Story",
		},
		{
			name:      "title_with_uppercase_prefix",
			epicTitle: "EPIC: My Story",
			wantName:  "EPIC: My Story",
		},
		{
			name:      "title_with_lowercase_prefix",
			epicTitle: "epic: my story",
			wantName:  "epic: my story",
		},
		{
			name:      "title_with_mixed_case_prefix",
			epicTitle: "Epic:Already",
			wantName:  "Epic:Already",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbDir := t.TempDir()
			dbPath := filepath.Join(dbDir, "test.db")

			if err := copyTemplateDB(dbPath); err != nil {
				t.Fatalf("failed to copy template DB: %v", err)
			}

			seedProject(t, dbPath, "test-proj")
			baseURL, client := startAPIServer(t, dbPath)

			database, err := db.OpenPath(dbPath)
			if err != nil {
				t.Fatalf("failed to open DB: %v", err)
			}
			defer database.Close()

			seedEpicWithChild(t, database, "test-proj", "epic-tc", tc.epicTitle, "child-tc")

			resp := runEpicRequest(t, client, baseURL, "test-proj", "epic-tc")
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
			}

			var result struct {
				Name string `json:"name"`
			}
			body, _ := io.ReadAll(resp.Body)
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if result.Name != tc.wantName {
				t.Errorf("chain name = %q, want %q", result.Name, tc.wantName)
			}
		})
	}
}
