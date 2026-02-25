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
)

// seedCategoryMessages seeds a project, session, and messages with varied categories.
func seedCategoryMessages(t *testing.T, dbPath, projectID, sessionID string) {
	t.Helper()

	seedProject(t, dbPath, projectID)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	ticketID := projectID + "-T1"

	if _, err := database.Exec(
		`INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Category Test Ticket", now, now, "test",
	); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if _, err := database.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"feature", projectID, "Feature", `["implementation"]`, now, now,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	wfiID := projectID + "-wfi"
	if _, err := database.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, projectID, ticketID, "feature", "active", `{}`, 0, now, now,
	); err != nil {
		t.Fatalf("seed wfi: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, projectID, ticketID, wfiID, "implementation", "implementor", "claude:sonnet", "running", now, now, now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	messages := []struct {
		content  string
		category string
	}{
		{"[Task] analyze codebase", "subagent"},
		{"[Bash] git status", "tool"},
		{"[TaskResult] general-purpose: analyze codebase", "subagent"},
		{"plain text response", "text"},
		{"[Skill] commit", "skill"},
		{"[Read] main.go", "tool"},
	}
	for i, m := range messages {
		ts := time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if _, err := database.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, ?, ?)`,
			sessionID, i, m.content, m.category, ts,
		); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	// Also seed a legacy message (no explicit category, uses DEFAULT 'text')
	legacySessionID := sessionID + "-legacy"
	if _, err := database.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		legacySessionID, projectID, ticketID, wfiID, "implementation", "implementor", "claude:sonnet", "running", now, now, now,
	); err != nil {
		t.Fatalf("seed legacy session: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
		legacySessionID, 0, "legacy message without category", now,
	); err != nil {
		t.Fatalf("seed legacy message: %v", err)
	}
}

// TestAgentLogE2E_Category is a comprehensive E2E test for message category support.
// It uses a single shared HTTP server to test all category-related API behaviors.
func TestAgentLogE2E_Category(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	const (
		projID       = "catproj-e2e"
		sessID       = "sess-cat-e2e"
		legacySessID = "sess-cat-e2e-legacy"
	)
	seedCategoryMessages(t, dbPath, projID, sessID)

	baseURL := startAPIServer(t, dbPath)

	t.Run("category_field_in_response", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?limit=100&offset=0", baseURL, sessID)
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
			Messages []struct {
				Content  string `json:"content"`
				Category string `json:"category"`
			} `json:"messages"`
			Total int `json:"total"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if result.Total != 6 {
			t.Fatalf("expected total=6, got %d", result.Total)
		}
		// Verify all messages have non-empty category
		for i, msg := range result.Messages {
			if msg.Category == "" {
				t.Errorf("message[%d]: missing category (content=%q)", i, msg.Content)
			}
		}
		// Verify specific categories
		wantCats := []string{"subagent", "tool", "subagent", "text", "skill", "tool"}
		for i, msg := range result.Messages {
			if msg.Category != wantCats[i] {
				t.Errorf("message[%d].Category = %q, want %q", i, msg.Category, wantCats[i])
			}
		}
	})

	t.Run("filter_subagent", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?category=subagent", baseURL, sessID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Messages []struct {
				Content  string `json:"content"`
				Category string `json:"category"`
			} `json:"messages"`
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		if result.Total != 2 {
			t.Fatalf("expected total=2 for subagent filter, got %d", result.Total)
		}
		for _, msg := range result.Messages {
			if msg.Category != "subagent" {
				t.Errorf("unexpected category %q for %q", msg.Category, msg.Content)
			}
		}
	})

	t.Run("filter_tool", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?category=tool", baseURL, sessID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		if result.Total != 2 {
			t.Fatalf("expected total=2 for tool filter, got %d", result.Total)
		}
	})

	t.Run("filter_no_category_returns_all", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Total int `json:"total"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		if result.Total != 6 {
			t.Fatalf("expected total=6 (no filter), got %d", result.Total)
		}
	})

	t.Run("legacy_message_defaults_to_text", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, legacySessID)
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Messages []struct {
				Content  string `json:"content"`
				Category string `json:"category"`
			} `json:"messages"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		if len(result.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result.Messages))
		}
		if result.Messages[0].Category != "text" {
			t.Errorf("Category = %q, want 'text' (DB DEFAULT for no-category inserts)", result.Messages[0].Category)
		}
	})
}
