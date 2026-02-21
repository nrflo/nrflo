package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/types"
)

// seedChainTickets inserts tickets directly into the DB for chain HTTP tests.
func seedChainTickets(t *testing.T, dbPath, projectID string, tickets map[string]time.Time) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding tickets: %v", err)
	}
	defer database.Close()

	for tid, createdAt := range tickets {
		created := createdAt.UTC().Format(time.RFC3339Nano)
		_, err := database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, ?, ?, 'test')`,
			strings.ToLower(tid), strings.ToLower(projectID), tid, created, created)
		if err != nil {
			t.Fatalf("failed to seed ticket %s: %v", tid, err)
		}
	}
}

// seedChainDeps inserts dependency rows directly into the DB.
func seedChainDeps(t *testing.T, dbPath, projectID string, deps map[string][]string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding deps: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for child, blockers := range deps {
		for _, blocker := range blockers {
			_, err := database.Exec(`
				INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_by, created_at)
				VALUES (?, ?, ?, 'blocks', 'test', ?)`,
				strings.ToLower(projectID), strings.ToLower(child), strings.ToLower(blocker), now)
			if err != nil {
				t.Fatalf("failed to seed dep %s->%s: %v", child, blocker, err)
			}
		}
	}
}

// doChainRequest is a helper to make authenticated chain API requests.
func doChainRequest(t *testing.T, method, url, projectID string, body interface{}) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if projectID != "" {
		req.Header.Set("X-Project", projectID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, url, err)
	}
	return resp
}

// TestChainPreviewEndpoint_HappyPath verifies POST /api/v1/chains/preview returns 200 with data.
func TestChainPreviewEndpoint_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const projectID = "proj-preview"
	seedProject(t, dbPath, projectID)

	base := time.Now()
	seedChainTickets(t, dbPath, projectID, map[string]time.Time{
		"T1": base,
		"T2": base.Add(time.Second),
		"T3": base.Add(2 * time.Second),
	})
	seedChainDeps(t, dbPath, projectID, map[string][]string{
		"T2": {"T1"},
		"T3": {"T2"},
	})

	baseURL := startAPIServer(t, dbPath)

	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains/preview", projectID,
		types.ChainPreviewRequest{TicketIDs: []string{"T3"}})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var preview types.ChainPreviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should expand T3 to include T1, T2, T3 in topological order
	if len(preview.TicketIDs) != 3 {
		t.Fatalf("expected 3 ticket_ids, got %d: %v", len(preview.TicketIDs), preview.TicketIDs)
	}
	if preview.TicketIDs[0] != "t1" || preview.TicketIDs[1] != "t2" || preview.TicketIDs[2] != "t3" {
		t.Errorf("expected [t1, t2, t3], got %v", preview.TicketIDs)
	}

	// Deps map should be present
	if preview.Deps == nil {
		t.Fatal("expected non-nil deps")
	}
	if len(preview.Deps["t2"]) == 0 {
		t.Errorf("expected deps[t2] to have entries, got %v", preview.Deps)
	}

	// T1 and T2 should be auto-added (only T3 was requested)
	if len(preview.AddedByDeps) != 2 {
		t.Errorf("expected 2 added_by_deps (T1, T2), got %d: %v", len(preview.AddedByDeps), preview.AddedByDeps)
	}
}

// TestChainPreviewEndpoint_NoTransitiveDeps verifies preview with no transitive additions.
func TestChainPreviewEndpoint_NoTransitiveDeps(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const projectID = "proj-nodeps"
	seedProject(t, dbPath, projectID)

	base := time.Now()
	seedChainTickets(t, dbPath, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	baseURL := startAPIServer(t, dbPath)

	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains/preview", projectID,
		types.ChainPreviewRequest{TicketIDs: []string{"A", "B"}})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var preview types.ChainPreviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(preview.TicketIDs) != 2 {
		t.Errorf("expected 2 ticket_ids, got %d", len(preview.TicketIDs))
	}
	if len(preview.AddedByDeps) != 0 {
		t.Errorf("expected empty added_by_deps, got %v", preview.AddedByDeps)
	}
}

// TestChainPreviewEndpoint_EmptyTickets verifies POST /api/v1/chains/preview with empty list returns 400.
func TestChainPreviewEndpoint_EmptyTickets(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj-empty")
	baseURL := startAPIServer(t, dbPath)

	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains/preview", "proj-empty",
		types.ChainPreviewRequest{TicketIDs: []string{}})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestChainPreviewEndpoint_MissingProjectHeader verifies 400 without X-Project header.
func TestChainPreviewEndpoint_MissingProjectHeader(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj-nohdr")
	baseURL := startAPIServer(t, dbPath)

	// No projectID → no X-Project header
	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains/preview", "",
		types.ChainPreviewRequest{TicketIDs: []string{"A"}})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 without X-Project, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestChainCreateEndpoint_WithCustomOrder verifies POST /api/v1/chains with ordered_ticket_ids.
func TestChainCreateEndpoint_WithCustomOrder(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const projectID = "proj-create"
	seedProject(t, dbPath, projectID)

	base := time.Now()
	seedChainTickets(t, dbPath, projectID, map[string]time.Time{
		"X": base,
		"Y": base.Add(time.Second),
		"Z": base.Add(2 * time.Second),
	})
	// No dependencies — any order valid

	baseURL := startAPIServer(t, dbPath)

	body := map[string]interface{}{
		"name":               "Custom Order Chain",
		"workflow_name":      "test",
		"ticket_ids":         []string{"X", "Y", "Z"},
		"ordered_ticket_ids": []string{"Z", "X", "Y"},
	}
	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains", projectID, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var chain map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&chain); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	items, ok := chain["items"].([]interface{})
	if !ok || len(items) != 3 {
		t.Fatalf("expected 3 items in chain response, got %v", chain["items"])
	}

	// Verify custom order: Z, X, Y
	expectedOrder := []string{"z", "x", "y"}
	for i, item := range items {
		m, _ := item.(map[string]interface{})
		if m["ticket_id"] != expectedOrder[i] {
			t.Errorf("item %d: expected ticket_id %s, got %v", i, expectedOrder[i], m["ticket_id"])
		}
	}
}

// TestChainCreateEndpoint_WithInvalidCustomOrder verifies 400 for invalid ordered_ticket_ids.
func TestChainCreateEndpoint_WithInvalidCustomOrder(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const projectID = "proj-invalord"
	seedProject(t, dbPath, projectID)

	base := time.Now()
	seedChainTickets(t, dbPath, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	seedChainDeps(t, dbPath, projectID, map[string][]string{
		"B": {"A"}, // B depends on A
	})

	baseURL := startAPIServer(t, dbPath)

	body := map[string]interface{}{
		"name":               "Invalid Order Chain",
		"workflow_name":      "test",
		"ticket_ids":         []string{"B"},
		"ordered_ticket_ids": []string{"b", "a"}, // invalid: B before A (its blocker)
	}
	resp := doChainRequest(t, "POST", baseURL+"/api/v1/chains", projectID, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for invalid order, got %d: %s", resp.StatusCode, string(respBody))
	}
}

// TestChainGetEndpoint_IncludesDeps verifies GET /api/v1/chains/{id} includes the deps field.
func TestChainGetEndpoint_IncludesDeps(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const projectID = "proj-getdeps"
	seedProject(t, dbPath, projectID)

	base := time.Now()
	seedChainTickets(t, dbPath, projectID, map[string]time.Time{
		"P": base,
		"Q": base.Add(time.Second),
	})
	seedChainDeps(t, dbPath, projectID, map[string][]string{
		"Q": {"P"}, // Q depends on P
	})

	baseURL := startAPIServer(t, dbPath)

	// Create a chain first
	createBody := map[string]interface{}{
		"name":          "Deps Chain",
		"workflow_name": "test",
		"ticket_ids":    []string{"Q"},
	}
	createResp := doChainRequest(t, "POST", baseURL+"/api/v1/chains", projectID, createBody)
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create chain: expected 201, got %d: %s", createResp.StatusCode, string(body))
	}

	var created map[string]interface{}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	chainID, _ := created["id"].(string)
	if chainID == "" {
		t.Fatal("expected chain ID in create response")
	}

	// Get the chain
	getResp := doChainRequest(t, "GET", fmt.Sprintf("%s/api/v1/chains/%s", baseURL, chainID), projectID, nil)
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 200, got %d: %s", getResp.StatusCode, string(body))
	}

	var chain map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&chain); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}

	// deps field should be present and contain Q -> P
	deps, ok := chain["deps"].(map[string]interface{})
	if !ok || deps == nil {
		t.Fatalf("expected deps in GET response, got %v", chain["deps"])
	}

	qDeps, ok := deps["q"].([]interface{})
	if !ok || len(qDeps) == 0 {
		t.Errorf("expected deps[q] to have P as blocker, got %v", deps["q"])
	} else if qDeps[0] != "p" {
		t.Errorf("expected deps[q]=[p], got %v", qDeps)
	}
}
