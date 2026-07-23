package handoff

import (
	"context"
	"strings"
	"testing"

	"be/internal/clock"

	"github.com/google/uuid"
)

func newIDs() (projectID, wfiID, sessionID string) {
	return uuid.New().String(), uuid.New().String(), uuid.New().String()
}

func TestSelectPlanFindings_TaskAnchorFromPrompt(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "Fix the login bug", "")

	hc, ok := resolveContext(context.Background(), pool, sessionID)
	if !ok {
		t.Fatalf("resolveContext failed")
	}
	if hc.taskAnchor != "Fix the login bug" {
		t.Errorf("taskAnchor = %q, want %q", hc.taskAnchor, "Fix the login bug")
	}
}

func TestSelectPlanFindings_KeepsPlanSuffixKeys_DropsOrchestrationKeys(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	seedFinding(t, pool, wfiID, sessionID, "implementor", "be_files_to_modify", "be/foo.go")
	seedFinding(t, pool, wfiID, sessionID, "planner", "fe_implementation_steps", "step one")
	seedFinding(t, pool, wfiID, sessionID, "implementor", "_orchestration_internal", "should be dropped")

	plan, _ := selectPlanFindings(pool, clock.Real(), wfiID, maxVerifiedBytes)
	joined := strings.Join(plan, "\n")
	if !strings.Contains(joined, "be_files_to_modify") {
		t.Errorf("plan = %v, want to contain be_files_to_modify", plan)
	}
	if !strings.Contains(joined, "fe_implementation_steps") {
		t.Errorf("plan = %v, want to contain fe_implementation_steps", plan)
	}
	if strings.Contains(joined, "_orchestration") {
		t.Errorf("plan = %v, must drop underscore-prefixed keys", plan)
	}
}

func TestSelectPlanFindings_WorkflowFinalResult_RendersAsOutcomeNotPlan(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	seedFinding(t, pool, wfiID, sessionID, "implementor", "workflow_final_result", "All tests pass")

	plan, outcome := selectPlanFindings(pool, clock.Real(), wfiID, maxVerifiedBytes)
	if outcome != "All tests pass" {
		t.Errorf("outcome = %q, want %q", outcome, "All tests pass")
	}
	for _, p := range plan {
		if strings.Contains(p, "workflow_final_result") {
			t.Errorf("plan = %v, workflow_final_result must not appear in Plan", plan)
		}
	}
}

func TestSelectPlanFindings_OversizedValue_TruncatedWithMarker(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	huge := strings.Repeat("x", maxFindingValueBytes+500)
	seedFinding(t, pool, wfiID, sessionID, "implementor", "be_files_to_modify", huge)

	plan, _ := selectPlanFindings(pool, clock.Real(), wfiID, maxVerifiedBytes)
	if len(plan) != 1 {
		t.Fatalf("plan = %v, want 1 line", plan)
	}
	if !strings.Contains(plan[0], "[truncated]") {
		t.Errorf("plan[0] = %q, want [truncated] marker", plan[0])
	}
	if len(plan[0]) >= len(huge) {
		t.Errorf("plan[0] length %d, want truncated below huge value length %d", len(plan[0]), len(huge))
	}
}

func TestSelectPlanFindings_DeterministicOrderingAcrossRuns(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	seedFinding(t, pool, wfiID, sessionID, "zeta", "be_files_to_modify", "z-value")
	seedFinding(t, pool, wfiID, sessionID, "alpha", "be_files_to_modify", "a-value")
	seedFinding(t, pool, wfiID, sessionID, "alpha", "fe_implementation_steps", "a2-value")

	plan1, _ := selectPlanFindings(pool, clock.Real(), wfiID, maxVerifiedBytes)
	plan2, _ := selectPlanFindings(pool, clock.Real(), wfiID, maxVerifiedBytes)

	if len(plan1) != len(plan2) {
		t.Fatalf("plan1 = %v, plan2 = %v, differing lengths", plan1, plan2)
	}
	for i := range plan1 {
		if plan1[i] != plan2[i] {
			t.Errorf("plan1[%d]=%q != plan2[%d]=%q, not deterministic", i, plan1[i], i, plan2[i])
		}
	}
	if len(plan1) < 2 || !strings.HasPrefix(plan1[0], "[alpha]") {
		t.Errorf("plan1 = %v, want alpha entries sorted before zeta", plan1)
	}
}

func TestSelectPlanFindings_BudgetStop_EmitsOmittedMarker(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, _ := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")

	// One session per agent — findings are unique per (scope_id, key), so
	// distinct sessions are needed to keep all three agent_type rows.
	for _, agentType := range []string{"alpha", "bravo", "charlie"} {
		sessionID := uuid.New().String()
		seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")
		seedFinding(t, pool, wfiID, sessionID, agentType, "be_files_to_modify", strings.Repeat(string(agentType[0]), 200))
	}

	// A tiny budget should admit the first entry then stop, leaving an
	// omitted-count marker rather than truncating every remaining line.
	plan, _ := selectPlanFindings(pool, clock.Real(), wfiID, 250)
	if len(plan) != 2 {
		t.Fatalf("plan = %v, want [1 entry, 1 omitted-marker line]", plan)
	}
	if !strings.HasPrefix(plan[0], "[alpha]") {
		t.Errorf("plan[0] = %q, want the alpha entry first (sorted)", plan[0])
	}
	if !strings.Contains(plan[1], "further plan findings omitted") {
		t.Errorf("plan[1] = %q, want an omitted-count marker", plan[1])
	}
}

func TestChainSessionIDs_AncestorWalk_StopsAtMaxChainSessions(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, _ := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")

	var ids []string
	for i := 0; i < maxChainSessions+2; i++ {
		ids = append(ids, uuid.New().String())
	}
	// Build chain: ids[0] (current/newest) -> ancestor ids[1] -> ids[2] -> ...
	// Insert every session first (no ancestor link yet) so the FOREIGN KEY on
	// ancestor_session_id always resolves once wired in the second pass.
	for _, id := range ids {
		seedSession(t, pool, id, projectID, wfiID, "node-1", "", "")
	}
	for i, id := range ids {
		if i+1 >= len(ids) {
			continue
		}
		if _, err := pool.Exec(`UPDATE agent_sessions SET ancestor_session_id = ? WHERE id = ?`, ids[i+1], id); err != nil {
			t.Fatalf("link ancestor: %v", err)
		}
	}

	chain := chainSessionIDs(context.Background(), pool, ids[0])
	if len(chain) != maxChainSessions {
		t.Fatalf("chain = %v, want length %d", chain, maxChainSessions)
	}
	for i := 0; i < maxChainSessions; i++ {
		if chain[i] != ids[i] {
			t.Errorf("chain[%d] = %q, want %q", i, chain[i], ids[i])
		}
	}
}
