package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/repo"
)

// seedSessionAndMessages opens the DB and creates the full chain:
// ticket → workflow def → workflow instance → agent session → messages.
func seedSessionAndMessages(t *testing.T, dbPath string) {
	t.Helper()

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	// Ticket (FK: projects)
	if _, err := database.Exec(`INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		"E2E-1", "e2eproj", "E2E Ticket", now, now, "test"); err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}

	// Workflow definition (FK: projects)
	if _, err := database.Exec(`INSERT OR IGNORE INTO workflows (id, project_id, description, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"feature", "e2eproj", "Feature workflow", `["implementation","verification"]`, now, now); err != nil {
		t.Fatalf("failed to seed workflow def: %v", err)
	}

	// Workflow instance (FK: workflow defs? none in schema — just project+ticket)
	if _, err := database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, current_phase, phase_order, phases, findings, retry_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wi-e2e-1", "e2eproj", "E2E-1", "feature", "active", "implementation",
		`["implementation","verification"]`, `{}`, `{}`, 0, now, now); err != nil {
		t.Fatalf("failed to seed workflow instance: %v", err)
	}

	// Agent session (FK: workflow_instances)
	if _, err := database.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-e2e-1", "e2eproj", "E2E-1", "wi-e2e-1", "implementation", "implementor", "claude:sonnet", "running", now, now, now); err != nil {
		t.Fatalf("failed to seed agent session: %v", err)
	}

	// Messages with timestamps 1 second apart
	baseTime := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	messages := []string{
		"[Read] src/main.ts — reading full file contents",
		"[Edit] src/utils.ts — replaced function signature",
		"[Bash] npm test -- running all test suites",
		"[Bash] " + makeString('x', 300), // >150 chars, old truncation limit
		"[Grep] searching for pattern across codebase",
		"[Write] created new test file",
		"[Glob] finding all *.test.ts files",
		"[Task] delegating subtask to subagent",
		"[WebFetch] fetching API documentation",
		"[WebSearch] searching for latest React docs",
		"[TodoWrite] updating task list",
		"[Skill] invoking commit skill",
		"plain message without tool prefix",
		"[UnknownTool] some exotic tool",
	}

	for i, msg := range messages {
		ts := baseTime.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		_, err := database.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-e2e-1", i, msg, ts,
		)
		if err != nil {
			t.Fatalf("failed to insert message %d: %v", i, err)
		}
	}
}

// TestAgentLogImprovements_E2E is an end-to-end test covering the backend portion of
// the "Improve active agent log" ticket via the HTTP API:
//
//  1. Full messages (no truncation) — long messages stored and returned completely
//  2. Timestamps on messages — API returns {content, created_at} objects
//  3. Pagination — limit/offset work correctly with timestamps
//  4. 404 for non-existent sessions
func TestAgentLogImprovements_E2E(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB (runs migrations)
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	// Seed data
	seedProject(t, dbPath, "e2eproj")
	seedSessionAndMessages(t, dbPath)

	// Start HTTP server
	baseURL := startAPIServer(t, dbPath)

	// -- Hit the messages endpoint --
	url := fmt.Sprintf("%s/api/v1/sessions/sess-e2e-1/messages?limit=100&offset=0", baseURL)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		SessionID string `json:"session_id"`
		Messages  []struct {
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"messages"`
		Total int `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify session_id
	if result.SessionID != "sess-e2e-1" {
		t.Fatalf("expected session_id=sess-e2e-1, got %q", result.SessionID)
	}

	// Verify total count
	if result.Total != 14 {
		t.Fatalf("expected total=14, got %d", result.Total)
	}

	// Verify message count
	if len(result.Messages) != 14 {
		t.Fatalf("expected 14 messages, got %d", len(result.Messages))
	}

	// Criterion 1: Full message content — long message NOT truncated
	longMsg := "[Bash] " + makeString('x', 300)
	if result.Messages[3].Content != longMsg {
		t.Errorf("message[3] truncated: expected len=%d, got len=%d", len(longMsg), len(result.Messages[3].Content))
	}

	// Criterion 2: All messages have valid RFC3339 timestamps
	baseTime := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	for i, msg := range result.Messages {
		if msg.CreatedAt == "" {
			t.Errorf("message[%d]: missing created_at timestamp", i)
			continue
		}
		ts, err := time.Parse(time.RFC3339, msg.CreatedAt)
		if err != nil {
			t.Errorf("message[%d]: invalid timestamp %q: %v", i, msg.CreatedAt, err)
			continue
		}
		expected := baseTime.Add(time.Duration(i) * time.Second)
		if !ts.Equal(expected) {
			t.Errorf("message[%d]: expected time %v, got %v", i, expected, ts)
		}
	}

	// -- Pagination test --
	url2 := fmt.Sprintf("%s/api/v1/sessions/sess-e2e-1/messages?limit=3&offset=0", baseURL)
	resp2, err := http.Get(url2)
	if err != nil {
		t.Fatalf("pagination request failed: %v", err)
	}
	defer resp2.Body.Close()

	var page1 struct {
		Messages []struct {
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"messages"`
		Total int `json:"total"`
	}
	json.NewDecoder(resp2.Body).Decode(&page1)

	if len(page1.Messages) != 3 {
		t.Fatalf("page1: expected 3 messages, got %d", len(page1.Messages))
	}
	if page1.Total != 14 {
		t.Fatalf("page1: expected total=14, got %d", page1.Total)
	}
	// Timestamps present on paginated results
	for i, msg := range page1.Messages {
		if msg.CreatedAt == "" {
			t.Errorf("page1 message[%d]: missing timestamp", i)
		}
	}

	// Page 2 has different content
	url3 := fmt.Sprintf("%s/api/v1/sessions/sess-e2e-1/messages?limit=3&offset=3", baseURL)
	resp3, err := http.Get(url3)
	if err != nil {
		t.Fatalf("page2 request failed: %v", err)
	}
	defer resp3.Body.Close()

	var page2 struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	json.NewDecoder(resp3.Body).Decode(&page2)

	if len(page2.Messages) != 3 {
		t.Fatalf("page2: expected 3 messages, got %d", len(page2.Messages))
	}
	if page2.Messages[0].Content == page1.Messages[0].Content {
		t.Fatal("page1 and page2 have the same first message — offset not working")
	}

	// -- 404 for non-existent session --
	url404 := fmt.Sprintf("%s/api/v1/sessions/does-not-exist/messages", baseURL)
	resp404, err := http.Get(url404)
	if err != nil {
		t.Fatalf("404 request failed: %v", err)
	}
	defer resp404.Body.Close()

	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent session, got %d", resp404.StatusCode)
	}
}

// TestAgentLogImprovements_MessageTimestamps_Service verifies that the service
// layer returns MessageWithTime objects with correct timestamps.
func TestAgentLogImprovements_MessageTimestamps_Service(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-ts-1", "TS-1")

	// Insert messages with known timestamps (varying intervals)
	t1 := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Second)
	t3 := t1.Add(5 * time.Second)

	for i, ts := range []time.Time{t1, t2, t3} {
		_, err := env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-ts-1", i, fmt.Sprintf("msg-%d", i), ts.Format(time.RFC3339),
		)
		if err != nil {
			t.Fatalf("insert msg %d: %v", i, err)
		}
	}

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-ts-1", 0, 0)
	if err != nil {
		t.Fatalf("GetSessionMessages: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}

	expectedTimes := []string{
		t1.Format(time.RFC3339),
		t2.Format(time.RFC3339),
		t3.Format(time.RFC3339),
	}
	for i, msg := range msgs {
		if msg.CreatedAt != expectedTimes[i] {
			t.Errorf("msg[%d]: expected created_at=%q, got %q", i, expectedTimes[i], msg.CreatedAt)
		}
		if msg.Content != fmt.Sprintf("msg-%d", i) {
			t.Errorf("msg[%d]: expected content=%q, got %q", i, fmt.Sprintf("msg-%d", i), msg.Content)
		}
	}
}

// TestAgentLogImprovements_BatchInsertPreservesTimestamps verifies that
// InsertBatch in the repo layer creates valid timestamps.
func TestAgentLogImprovements_BatchInsertPreservesTimestamps(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-bi-1", "BI-1")

	msgRepo := repo.NewAgentMessagePoolRepo(env.Pool)
	err := msgRepo.InsertBatch("sess-bi-1", 0, []string{
		"[Bash] git status",
		"[Read] main.go",
		"[Edit] handler.go",
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	msgs, err := msgRepo.GetBySessionPaginated("sess-bi-1", 100, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	for i, msg := range msgs {
		expected := []string{"[Bash] git status", "[Read] main.go", "[Edit] handler.go"}
		if msg.Content != expected[i] {
			t.Errorf("msg[%d]: content=%q, want %q", i, msg.Content, expected[i])
		}

		if msg.CreatedAt == "" {
			t.Errorf("msg[%d]: empty created_at", i)
			continue
		}
		_, err := time.Parse(time.RFC3339, msg.CreatedAt)
		if err != nil {
			t.Errorf("msg[%d]: invalid timestamp %q: %v", i, msg.CreatedAt, err)
		}
	}

	// All messages in same batch should have the same timestamp
	if msgs[0].CreatedAt != msgs[1].CreatedAt || msgs[1].CreatedAt != msgs[2].CreatedAt {
		t.Errorf("batch messages should have same timestamp, got %q, %q, %q",
			msgs[0].CreatedAt, msgs[1].CreatedAt, msgs[2].CreatedAt)
	}
}

// TestAgentLogImprovements_FullMessageNoTruncation verifies that very long
// messages are stored and returned in full.
func TestAgentLogImprovements_FullMessageNoTruncation(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-full-1", "FULL-1")

	shortMsg := "[Read] file.ts"
	mediumMsg := "[Bash] npm run test -- --coverage --verbose --watchAll=false --forceExit --detectOpenHandles"
	longMsg := "[Edit] " + makeString('a', 200)
	veryLongMsg := "[Bash] " + makeString('b', 500)

	now := time.Now().UTC().Format(time.RFC3339)
	for i, msg := range []string{shortMsg, mediumMsg, longMsg, veryLongMsg} {
		_, err := env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-full-1", i, msg, now,
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-full-1", 0, 0)
	if err != nil {
		t.Fatalf("GetSessionMessages: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected total=4, got %d", total)
	}

	expectedLengths := []int{len(shortMsg), len(mediumMsg), len(longMsg), len(veryLongMsg)}
	for i, msg := range msgs {
		if len(msg.Content) != expectedLengths[i] {
			t.Errorf("msg[%d]: expected len=%d, got len=%d", i, expectedLengths[i], len(msg.Content))
		}
	}
}

func makeString(ch byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
