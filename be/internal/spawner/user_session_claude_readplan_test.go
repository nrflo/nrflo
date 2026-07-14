package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── readPlanFile (migrated from orchestrator/plan_reader_test.go) ──────────

// planReaderTestEnv sets up a fake HOME directory for plan reader tests.
// It returns the plans dir path and a helper to write session JSONL logs.
func planReaderTestEnv(t *testing.T) (plansDir string, writeLog func(sessionID, projectRoot, content string)) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	plansDir = filepath.Join(homeDir, ".claude", "plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	writeLog = func(sessionID, projectRoot, content string) {
		t.Helper()
		encodedRoot := "-" + strings.ReplaceAll(strings.TrimPrefix(projectRoot, "/"), "/", "-")
		logDir := filepath.Join(homeDir, ".claude", "projects", encodedRoot)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatalf("failed to create log dir: %v", err)
		}
		logPath := filepath.Join(logDir, sessionID+".jsonl")
		if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write session log: %v", err)
		}
	}
	return plansDir, writeLog
}

func TestReadPlanFile_NoPlansDirReturnsEmpty(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	// No plans dir created

	result := readPlanFile("any-session", "/some/project")
	if result != "" {
		t.Errorf("readPlanFile() = %q, want empty string when plans dir missing", result)
	}
}

func TestReadPlanFile_EmptyPlansDirReturnsEmpty(t *testing.T) {
	plansDir, _ := planReaderTestEnv(t)
	os.WriteFile(filepath.Join(plansDir, "notes.txt"), []byte("not a plan"), 0644)

	result := readPlanFile("any-session", "/some/project")
	if result != "" {
		t.Errorf("readPlanFile() = %q, want empty string with no .md files", result)
	}
}

func TestReadPlanFile_OldFilesExcluded(t *testing.T) {
	plansDir, _ := planReaderTestEnv(t)

	oldPlan := filepath.Join(plansDir, "old.md")
	os.WriteFile(oldPlan, []byte("old plan content"), 0644)
	oldTime := time.Now().Add(-72 * time.Hour)
	os.Chtimes(oldPlan, oldTime, oldTime)

	result := readPlanFile("any-session", "/some/project")
	if result != "" {
		t.Errorf("readPlanFile() = %q, want empty string for old plan files", result)
	}
}

func TestReadPlanFile_NoSessionLogReturnsEmpty(t *testing.T) {
	plansDir, _ := planReaderTestEnv(t)

	os.WriteFile(filepath.Join(plansDir, "plan.md"), []byte("plan content"), 0644)

	result := readPlanFile("missing-session", "/some/project")
	if result != "" {
		t.Errorf("readPlanFile() = %q, want empty string when session log missing", result)
	}
}

func TestReadPlanFile_MatchingPlanFound(t *testing.T) {
	plansDir, writeLog := planReaderTestEnv(t)

	planContent := "# Implementation Plan\n\nStep 1: Do thing\nStep 2: Do other thing"
	planFile := "my-plan.md"
	os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0644)

	sessionID := "test-session-123"
	projectRoot := "/Users/test/myproject"
	logContent := fmt.Sprintf(`{"type":"assistant","message":"creating plan %s"}`, planFile)
	writeLog(sessionID, projectRoot, logContent)

	result := readPlanFile(sessionID, projectRoot)
	if result != planContent {
		t.Errorf("readPlanFile() = %q, want %q", result, planContent)
	}
}

func TestReadPlanFile_ProjectRootEncoding(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	plansDir := filepath.Join(homeDir, ".claude", "plans")
	os.MkdirAll(plansDir, 0755)

	planContent := "encoded root plan"
	os.WriteFile(filepath.Join(plansDir, "enc-plan.md"), []byte(planContent), 0644)

	sessionID := "enc-session-456"
	projectRoot := "/Users/foo/bar/project"
	encodedRoot := "-Users-foo-bar-project"
	logDir := filepath.Join(homeDir, ".claude", "projects", encodedRoot)
	os.MkdirAll(logDir, 0755)
	os.WriteFile(filepath.Join(logDir, sessionID+".jsonl"), []byte("mentioned enc-plan.md here"), 0644)

	result := readPlanFile(sessionID, projectRoot)
	if result != planContent {
		t.Errorf("readPlanFile() = %q, want %q (path encoding may be wrong)", result, planContent)
	}
}

func TestReadPlanFile_MultipleMatchesUsesLast(t *testing.T) {
	plansDir, writeLog := planReaderTestEnv(t)

	os.WriteFile(filepath.Join(plansDir, "plan-a.md"), []byte("content of plan a"), 0644)
	os.WriteFile(filepath.Join(plansDir, "plan-b.md"), []byte("content of plan b"), 0644)

	sessionID := "multi-match-session"
	projectRoot := "/multi/project"
	logContent := "{\"msg\":\"draft plan-a.md\"}\n{\"msg\":\"final plan-b.md\"}\n"
	writeLog(sessionID, projectRoot, logContent)

	result := readPlanFile(sessionID, projectRoot)
	if result != "content of plan b" {
		t.Errorf("readPlanFile() = %q, want 'content of plan b' (last match in session log)", result)
	}
}

func TestReadPlanFile_SessionLogWithNoPlanMentionReturnsEmpty(t *testing.T) {
	plansDir, writeLog := planReaderTestEnv(t)

	os.WriteFile(filepath.Join(plansDir, "plan.md"), []byte("plan content"), 0644)

	sessionID := "no-mention-session"
	projectRoot := "/no/mention/project"
	writeLog(sessionID, projectRoot, "{\"msg\":\"doing some work, no plans here\"}\n")

	result := readPlanFile(sessionID, projectRoot)
	if result != "" {
		t.Errorf("readPlanFile() = %q, want empty string when plan not mentioned in session log", result)
	}
}

// TestClaudeAdapter_ReadPlan_DelegatesToReadPlanFile verifies ReadPlan wires
// SessionID/WorkDir through to readPlanFile.
func TestClaudeAdapter_ReadPlan_DelegatesToReadPlanFile(t *testing.T) {
	plansDir, writeLog := planReaderTestEnv(t)

	planContent := "# Plan via adapter"
	os.WriteFile(filepath.Join(plansDir, "adapter-plan.md"), []byte(planContent), 0644)

	sessionID := "adapter-session"
	projectRoot := "/adapter/project"
	writeLog(sessionID, projectRoot, `{"msg":"see adapter-plan.md"}`)

	adapter := &ClaudeAdapter{}
	result := adapter.ReadPlan(PlanCaptureOptions{SessionID: sessionID, WorkDir: projectRoot})
	if result != planContent {
		t.Errorf("ReadPlan() = %q, want %q", result, planContent)
	}
}
