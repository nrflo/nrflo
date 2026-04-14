package spawner

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

func TestIsContinuationReason(t *testing.T) {
	tests := []struct {
		reason string
		want   bool
	}{
		{"stall_restart_start_stall", true},
		{"stall_restart_running_stall", true},
		{"instant_stall", true},
		{"fail_restart", true},
		{"timeout_restart", true},
		{"low_context", false},
		{"explicit", false},
		{"implicit", false},
		{"", false},
		{"stall_budget_exhausted", false},
		{"pass", false},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if got := isContinuationReason(tt.reason); got != tt.want {
				t.Errorf("isContinuationReason(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestExpandInjectable_BasicExpansion(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	body := sp.expandInjectable("user-instructions", map[string]string{
		"USER_INSTRUCTIONS": "Fix the login bug",
	})
	if !strings.Contains(body, "Fix the login bug") {
		t.Errorf("expected instructions in body, got %q", body)
	}
	if strings.Contains(body, "${USER_INSTRUCTIONS}") {
		t.Error("${USER_INSTRUCTIONS} placeholder not expanded")
	}
}

func TestExpandInjectable_MissingFromDB(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	body := sp.expandInjectable("nonexistent-injectable", nil)
	if body != "" {
		t.Errorf("expected empty for missing injectable, got %q", body)
	}
}

func TestExpandInjectable_StripsUnusedPlaceholders(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	body := sp.expandInjectable("callback", map[string]string{
		"CALLBACK_INSTRUCTIONS": "Fix auth",
	})
	if strings.Contains(body, "${CALLBACK_FROM_AGENT}") {
		t.Error("unused ${CALLBACK_FROM_AGENT} should be stripped")
	}
	if !strings.Contains(body, "Fix auth") {
		t.Errorf("expected expanded instructions, got %q", body)
	}
}

func TestExpandInjectable_NilVars(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	body := sp.expandInjectable("continuation", nil)
	if body == "" {
		t.Error("expected non-empty continuation body")
	}
	if strings.Contains(body, "${") {
		t.Error("body should not contain ${...} placeholders")
	}
}

func TestLoadTemplate_LegacyPlaceholderStripping(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "LP-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer",
		"A:${USER_INSTRUCTIONS}:B:${CALLBACK_INSTRUCTIONS}:C:${PREVIOUS_DATA}:D")

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if strings.Contains(result, "${USER_INSTRUCTIONS}") ||
		strings.Contains(result, "${CALLBACK_INSTRUCTIONS}") ||
		strings.Contains(result, "${PREVIOUS_DATA}") {
		t.Error("legacy placeholders were not stripped")
	}
	if !strings.Contains(result, "A::B::C::D") {
		t.Errorf("expected stripped body 'A::B::C::D', got: %s", result)
	}
}

func TestLoadTemplate_UserInstructionsPrepended(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "UI-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"user_instructions": "Focus on auth",
	})

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "## User Instructions") {
		t.Error("expected user-instructions header")
	}
	if !strings.Contains(result, "Focus on auth") {
		t.Error("expected user instructions content")
	}
	uiIdx := strings.Index(result, "## User Instructions")
	bodyIdx := strings.Index(result, "Main prompt body")
	if uiIdx >= bodyIdx {
		t.Error("user instructions should be prepended before main body")
	}
}

func TestLoadTemplate_UserInstructionsAbsent_NoPrepend(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "UI-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if strings.Contains(result, "## User Instructions") {
		t.Error("user-instructions should not be prepended when absent")
	}
}

func TestLoadTemplate_InjectableMissingFromDB(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "IM-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main body")

	_, err := env.pool.Exec(`DELETE FROM default_templates WHERE type = 'injectable'`)
	if err != nil {
		t.Fatalf("failed to delete injectable templates: %v", err)
	}

	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"user_instructions": "This should trigger lookup",
	})

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate should not crash with missing injectable: %v", err)
	}
	if !strings.Contains(result, "Main body") {
		t.Error("expected main body to still be present")
	}
}

// createContinuedSessionInEnv creates a continued agent session in a spawnerTestEnv.
func createContinuedSessionInEnv(t *testing.T, env *spawnerTestEnv, ticketID, wfiID, agentType, modelID, phase, resultReason string, findings map[string]interface{}) {
	t.Helper()
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	session := &model.AgentSession{
		ID:                 uuid.New().String(),
		ProjectID:          env.project,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		Findings:           sql.NullString{String: string(findingsJSON), Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
		EndedAt:            sql.NullString{String: now, Valid: true},
	}
	if resultReason != "" {
		session.ResultReason = sql.NullString{String: resultReason, Valid: true}
	}
	sessionRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}
}
