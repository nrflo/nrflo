package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestGetTicketWithParentAndSiblings(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "hierarchy-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create parent epic
	epicBody := `{"id":"EPIC-001","title":"Parent Epic","issue_type":"epic","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "hierarchy-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()

	// Create first child (the one we'll query)
	child1Body := `{"id":"CHILD-001","title":"First Child","parent_ticket_id":"EPIC-001","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child1Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "hierarchy-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 1: %v", err)
	}
	resp.Body.Close()

	// Create second child (sibling)
	child2Body := `{"id":"CHILD-002","title":"Second Child","parent_ticket_id":"EPIC-001","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child2Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "hierarchy-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 2: %v", err)
	}
	resp.Body.Close()

	// Create third child (another sibling)
	child3Body := `{"id":"CHILD-003","title":"Third Child","parent_ticket_id":"EPIC-001","priority":3,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child3Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "hierarchy-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 3: %v", err)
	}
	resp.Body.Close()

	// GET the first child and verify parent_ticket and siblings
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/CHILD-001", nil)
	req.Header.Set("X-Project", "hierarchy-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID           string          `json:"id"`
		Title        string          `json:"title"`
		ParentTicket *model.Ticket   `json:"parent_ticket"`
		Siblings     []*model.Ticket `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify parent_ticket is populated
	if result.ParentTicket == nil {
		t.Fatalf("expected parent_ticket to be populated, got nil")
	}
	if result.ParentTicket.ID != "epic-001" {
		t.Errorf("expected parent_ticket.id 'epic-001', got %q", result.ParentTicket.ID)
	}
	if result.ParentTicket.Title != "Parent Epic" {
		t.Errorf("expected parent_ticket.title 'Parent Epic', got %q", result.ParentTicket.Title)
	}

	// Verify siblings are populated (should exclude current ticket)
	if len(result.Siblings) != 2 {
		t.Fatalf("expected 2 siblings, got %d", len(result.Siblings))
	}

	// Verify current ticket is not in siblings
	for _, s := range result.Siblings {
		if s.ID == "child-001" {
			t.Errorf("current ticket should not be in siblings list")
		}
	}

	// Verify siblings are ordered by priority ASC
	if result.Siblings[0].ID != "child-002" {
		t.Errorf("expected first sibling to be child-002 (priority 1), got %q", result.Siblings[0].ID)
	}
	if result.Siblings[1].ID != "child-003" {
		t.Errorf("expected second sibling to be child-003 (priority 3), got %q", result.Siblings[1].ID)
	}
}

func TestGetTicketWithoutParent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "no-parent-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create standalone ticket
	ticketBody := `{"id":"STANDALONE-001","title":"Standalone Ticket","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(ticketBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "no-parent-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
	resp.Body.Close()

	// GET the ticket
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/STANDALONE-001", nil)
	req.Header.Set("X-Project", "no-parent-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID           string          `json:"id"`
		ParentTicket *model.Ticket   `json:"parent_ticket"`
		Siblings     []*model.Ticket `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify parent_ticket is null
	if result.ParentTicket != nil {
		t.Errorf("expected parent_ticket to be null for ticket without parent, got %+v", result.ParentTicket)
	}

	// Verify siblings is empty array
	if result.Siblings == nil {
		t.Fatalf("expected siblings to be empty array [], got nil")
	}
	if len(result.Siblings) != 0 {
		t.Errorf("expected 0 siblings for ticket without parent, got %d", len(result.Siblings))
	}
}

func TestGetTicketWithDeletedParent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	seedProject(t, dbPath, "deleted-parent-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create parent epic
	epicBody := `{"id":"EPIC-DEL","title":"Epic to Delete","issue_type":"epic","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deleted-parent-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()

	// Create child
	childBody := `{"id":"CHILD-ORPHAN","title":"Orphan Child","parent_ticket_id":"EPIC-DEL","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(childBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deleted-parent-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child: %v", err)
	}
	resp.Body.Close()

	// Delete parent directly from DB
	openDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	_, err = openDB.Exec(`DELETE FROM tickets WHERE LOWER(id) = LOWER(?) AND LOWER(project_id) = LOWER(?)`, "epic-del", "deleted-parent-proj")
	openDB.Close()
	if err != nil {
		t.Fatalf("failed to delete parent: %v", err)
	}

	// GET the child
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/CHILD-ORPHAN", nil)
	req.Header.Set("X-Project", "deleted-parent-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 (handler should log warning but not fail), got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID           string          `json:"id"`
		ParentTicket *model.Ticket   `json:"parent_ticket"`
		Siblings     []*model.Ticket `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Parent fetch should fail gracefully - parent_ticket omitted (null)
	if result.ParentTicket != nil {
		t.Errorf("expected parent_ticket to be null when parent is deleted, got %+v", result.ParentTicket)
	}

	// Siblings should be empty array (parent doesn't exist, so no siblings to fetch)
	if result.Siblings == nil {
		t.Fatalf("expected siblings to be empty array [], got nil")
	}
	if len(result.Siblings) != 0 {
		t.Errorf("expected 0 siblings when parent is deleted, got %d", len(result.Siblings))
	}
}

func TestGetTicketSiblingsCaseInsensitive(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "case-siblings")
	baseURL := startAPIServer(t, dbPath)

	// Create parent with mixed case
	epicBody := `{"id":"MixedCase-Epic","title":"Parent Epic","issue_type":"epic","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "case-siblings")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()

	// Create children with different case in ID
	child1Body := `{"id":"Child-One","title":"First Child","parent_ticket_id":"MIXEDCASE-EPIC","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child1Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "case-siblings")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 1: %v", err)
	}
	resp.Body.Close()

	child2Body := `{"id":"CHILD-TWO","title":"Second Child","parent_ticket_id":"mixedcase-epic","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child2Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "case-siblings")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 2: %v", err)
	}
	resp.Body.Close()

	// GET first child with different case
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/CHILD-ONE", nil)
	req.Header.Set("X-Project", "case-siblings")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		ID       string          `json:"id"`
		Siblings []*model.Ticket `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify siblings (should exclude current ticket case-insensitively)
	if len(result.Siblings) != 1 {
		t.Fatalf("expected 1 sibling (case-insensitive exclusion), got %d", len(result.Siblings))
	}
	if result.Siblings[0].ID != "child-two" {
		t.Errorf("expected sibling 'child-two', got %q", result.Siblings[0].ID)
	}
}

func TestGetTicketWithDependenciesAndTitles(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	seedProject(t, dbPath, "deps-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create blocker ticket
	blockerBody := `{"id":"BLOCKER-001","title":"Blocker Ticket","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(blockerBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deps-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}
	resp.Body.Close()

	// Create blocked ticket
	blockedBody := `{"id":"BLOCKED-001","title":"Blocked Ticket","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(blockedBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deps-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create blocked: %v", err)
	}
	resp.Body.Close()

	// Create ticket that gets blocked by blocked ticket
	blockeeBody := `{"id":"BLOCKEE-001","title":"Blockee Ticket","priority":3,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(blockeeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deps-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create blockee: %v", err)
	}
	resp.Body.Close()

	// Add dependency: BLOCKED-001 depends on BLOCKER-001
	depBody := `{"issue_id":"BLOCKED-001","depends_on_id":"BLOCKER-001"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/dependencies", bytes.NewBufferString(depBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deps-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to add dependency: %v", err)
	}
	resp.Body.Close()

	// Add dependency: BLOCKEE-001 depends on BLOCKED-001
	depBody2 := `{"issue_id":"BLOCKEE-001","depends_on_id":"BLOCKED-001"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/dependencies", bytes.NewBufferString(depBody2))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "deps-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to add dependency 2: %v", err)
	}
	resp.Body.Close()

	// GET the blocked ticket and verify dependency titles
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/BLOCKED-001", nil)
	req.Header.Set("X-Project", "deps-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		ID       string               `json:"id"`
		Blockers []*model.Dependency  `json:"blockers"`
		Blocks   []*model.Dependency  `json:"blocks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify blockers have titles
	if len(result.Blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(result.Blockers))
	}
	if result.Blockers[0].DependsOnTitle != "Blocker Ticket" {
		t.Errorf("expected depends_on_title 'Blocker Ticket', got %q", result.Blockers[0].DependsOnTitle)
	}

	// Verify blocks have titles
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 blocked ticket, got %d", len(result.Blocks))
	}
	if result.Blocks[0].IssueTitle != "Blockee Ticket" {
		t.Errorf("expected issue_title 'Blockee Ticket', got %q", result.Blocks[0].IssueTitle)
	}
}

func TestGetEpicWithChildrenAndSiblings(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "epic-child-sib")
	baseURL := startAPIServer(t, dbPath)

	// Create parent meta-epic
	metaEpicBody := `{"id":"META-EPIC","title":"Meta Epic","issue_type":"epic","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(metaEpicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-child-sib")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create meta epic: %v", err)
	}
	resp.Body.Close()

	// Create child epic (has both parent and children)
	childEpicBody := `{"id":"CHILD-EPIC","title":"Child Epic","issue_type":"epic","parent_ticket_id":"META-EPIC","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(childEpicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-child-sib")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child epic: %v", err)
	}
	resp.Body.Close()

	// Create sibling epic
	siblingEpicBody := `{"id":"SIBLING-EPIC","title":"Sibling Epic","issue_type":"epic","parent_ticket_id":"META-EPIC","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(siblingEpicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-child-sib")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create sibling epic: %v", err)
	}
	resp.Body.Close()

	// Create grandchild under child epic
	grandchildBody := `{"id":"GRANDCHILD","title":"Grandchild Task","parent_ticket_id":"CHILD-EPIC","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(grandchildBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-child-sib")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create grandchild: %v", err)
	}
	resp.Body.Close()

	// GET the child epic
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/CHILD-EPIC", nil)
	req.Header.Set("X-Project", "epic-child-sib")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get child epic: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		ID           string          `json:"id"`
		IssueType    string          `json:"issue_type"`
		ParentTicket *model.Ticket   `json:"parent_ticket"`
		Siblings     []*model.Ticket `json:"siblings"`
		Children     []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify it's an epic
	if result.IssueType != "epic" {
		t.Fatalf("expected issue_type 'epic', got %q", result.IssueType)
	}

	// Verify parent_ticket
	if result.ParentTicket == nil {
		t.Fatalf("expected parent_ticket to be populated")
	}
	if result.ParentTicket.ID != "meta-epic" {
		t.Errorf("expected parent_ticket.id 'meta-epic', got %q", result.ParentTicket.ID)
	}

	// Verify siblings (should have sibling epic)
	if len(result.Siblings) != 1 {
		t.Fatalf("expected 1 sibling, got %d", len(result.Siblings))
	}
	if result.Siblings[0].ID != "sibling-epic" {
		t.Errorf("expected sibling 'sibling-epic', got %q", result.Siblings[0].ID)
	}

	// Verify children (should have grandchild)
	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(result.Children))
	}
	if result.Children[0].ID != "grandchild" {
		t.Errorf("expected child 'grandchild', got %q", result.Children[0].ID)
	}
}
