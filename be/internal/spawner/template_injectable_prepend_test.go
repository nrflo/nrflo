package spawner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLoadTemplate_LowContextPrepended(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "LC-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	createContinuedSessionInEnv(t, env, ticketID, wfiID,
		"analyzer", "claude:sonnet", "test-phase", "low_context",
		map[string]interface{}{"to_resume": "saved progress data"})

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "test-phase", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "## Continuation From Saved State") {
		t.Error("expected low-context injectable header")
	}
	if !strings.Contains(result, "saved progress data") {
		t.Error("expected PREVIOUS_DATA expanded in low-context block")
	}
	if strings.Contains(result, "${PREVIOUS_DATA}") {
		t.Error("${PREVIOUS_DATA} placeholder should be stripped")
	}
}

func TestLoadTemplate_ContinuationReasonNoPrepend(t *testing.T) {
	reasons := []string{
		"stall_restart",
		"stall_restart_start_stall",
		"stall_restart_running_stall",
		"fail_restart",
		"timeout_restart",
		"instant_stall",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			env := newSpawnerTestEnv(t)
			ticketID := "CN-" + uuid.New().String()[:6]
			env.initWorkflow(t, ticketID)
			createAgentDef(t, env, "analyzer", "Main prompt body")

			wfiID := env.getWfiID(t, ticketID)
			createContinuedSessionInEnv(t, env, ticketID, wfiID,
				"analyzer", "claude:sonnet", "test-phase", reason,
				map[string]interface{}{})

			sp := env.newSpawner()
			result, err := sp.loadTemplate("analyzer", ticketID, env.project,
				"p", "c", "test", "claude:sonnet", "test-phase", "", nil)
			if err != nil {
				t.Fatalf("loadTemplate failed: %v", err)
			}
			if strings.Contains(result, "## Continuation") {
				t.Error("no continuation block should be prepended after injectable removal")
			}
			if !strings.HasPrefix(result, "Main prompt body") {
				t.Error("main body should be at start of result (no prepended blocks)")
			}
		})
	}
}

func TestLoadTemplate_PrependOrdering(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "PO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"user_instructions": "User context here",
		"_callback": map[string]interface{}{
			"instructions": "Callback action here",
			"from_agent":   "qa-verifier",
			"level":        0,
		},
	})
	createContinuedSessionInEnv(t, env, ticketID, wfiID,
		"analyzer", "claude:sonnet", "test-phase", "low_context",
		map[string]interface{}{"to_resume": "saved state"})

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "test-phase", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	uiIdx := strings.Index(result, "## User Instructions")
	lcIdx := strings.Index(result, "## Continuation From Saved State")
	cbIdx := strings.Index(result, "## Callback Instructions")
	bodyIdx := strings.Index(result, "Main prompt body")
	if uiIdx == -1 {
		t.Fatal("missing user-instructions block")
	}
	if lcIdx == -1 {
		t.Fatal("missing low-context block")
	}
	if cbIdx == -1 {
		t.Fatal("missing callback block")
	}
	if uiIdx >= lcIdx {
		t.Errorf("user-instructions (idx=%d) should come before low-context (idx=%d)", uiIdx, lcIdx)
	}
	if lcIdx >= cbIdx {
		t.Errorf("low-context (idx=%d) should come before callback (idx=%d)", lcIdx, cbIdx)
	}
	if cbIdx >= bodyIdx {
		t.Errorf("callback (idx=%d) should come before main body (idx=%d)", cbIdx, bodyIdx)
	}
}
