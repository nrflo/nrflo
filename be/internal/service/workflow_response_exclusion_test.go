package service

import (
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// The v4 read model hides system/internal sessions via the shared
// transientAgentTypeExclusion SQL clause (workflow_response.go), which has three
// branches: named system agents (planner/context-saver/conflict-resolver),
// underscore-prefixed agent_type, and underscore-prefixed phase. These tests verify
// every branch is excluded from buildActiveAgentsMap, buildAgentHistory, and
// derivePhaseStatuses, while legitimate agents survive.

// insertSessionWithPhase inserts an agent_session where phase differs from agent_type.
// Used to exercise the `phase NOT LIKE '\_%' ESCAPE '\'` branch of the exclusion clause.
func insertSessionWithPhase(t *testing.T, pool *db.Pool, id, wfiID, agentType, phase, status, result string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var resultVal interface{}
	if result != "" {
		resultVal = result
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 status, result, result_reason, pid, context_left, ancestor_session_id,
			 spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		id, wfiID, phase, agentType, status, resultVal, now, now, now)
	if err != nil {
		t.Fatalf("insertSessionWithPhase %s (phase=%s agent_type=%s): %v", id, phase, agentType, err)
	}
}

// mapKeys returns the keys of a map[string]interface{} for error messages.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// excludedSessionCase describes a single transient/system session and which exclusion
// branch it exercises.
type excludedSessionCase struct {
	name      string
	agentType string
	phase     string // if "" the session phase equals agentType
}

// transientCases enumerates one row per distinct exclusion mechanism. Keep these
// distinct: named system agents, underscore agent_type, and underscore phase each
// hit a different sub-clause of transientAgentTypeExclusion.
var transientCases = []excludedSessionCase{
	{name: "named_planner", agentType: "planner"},
	{name: "named_context_saver", agentType: "context-saver"},
	{name: "named_conflict_resolver", agentType: "conflict-resolver"},
	{name: "underscore_agent_type_consult", agentType: "_consult"},
	{name: "underscore_agent_type_finalize", agentType: "_finalize"},
	{name: "underscore_agent_type_pause", agentType: "_pause"},
	{name: "underscore_agent_type_notification", agentType: "_notification"},
	{name: "underscore_phase_consult", agentType: "architect", phase: "_consult"},
}

// insertExcluded inserts the case's transient session with the given status/result.
func (c excludedSessionCase) insert(t *testing.T, pool *db.Pool, id, wfiID, status, result string) {
	t.Helper()
	if c.phase != "" {
		insertSessionWithPhase(t, pool, id, wfiID, c.agentType, c.phase, status, result)
		return
	}
	insertSession(t, pool, id, wfiID, c.agentType, status, result, "")
}

// TestTransientSessionsExcluded verifies each exclusion-clause branch removes the
// transient session from all three read-model functions while a normal "analyzer"
// session is still surfaced.
func TestTransientSessionsExcluded(t *testing.T) {
	t.Parallel()

	for _, tc := range transientCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Active agents + phase statuses: excluded + normal both running.
			t.Run("active_and_phase", func(t *testing.T) {
				pool, svc, wfiID := setupDeriveTestEnv(t)
				tc.insert(t, pool, "s-excluded", wfiID, "running", "")
				insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

				active := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
				if _, ok := active[tc.agentType]; ok {
					t.Errorf("%s must not appear in buildActiveAgentsMap; keys: %v", tc.agentType, mapKeys(active))
				}
				if _, ok := active["analyzer"]; !ok {
					t.Errorf("analyzer must appear in buildActiveAgentsMap; keys: %v", mapKeys(active))
				}
				if len(active) != 1 {
					t.Errorf("buildActiveAgentsMap len = %d, want 1 (only analyzer); keys: %v", len(active), mapKeys(active))
				}

				phases := svc.derivePhaseStatuses(wfiID, twoPhases)
				if _, ok := phases[tc.agentType]; ok && tc.agentType != "analyzer" {
					t.Errorf("%s must not appear as a phase key in derivePhaseStatuses", tc.agentType)
				}
				if _, ok := phases[tc.phase]; tc.phase != "" && ok {
					t.Errorf("phase %q must not appear as a phase key in derivePhaseStatuses", tc.phase)
				}
				assertPhase(t, phases, "analyzer", "in_progress", "")
				assertPhase(t, phases, "builder", "pending", "")
			})

			// History: excluded + normal both terminal.
			t.Run("history", func(t *testing.T) {
				pool, svc, wfiID := setupDeriveTestEnv(t)
				tc.insert(t, pool, "s-excluded", wfiID, "completed", "pass")
				insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "")

				history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
				if len(history) != 1 {
					t.Fatalf("buildAgentHistory len = %d, want 1 (transient excluded)", len(history))
				}
				entry, ok := history[0].(map[string]interface{})
				if !ok {
					t.Fatalf("history[0] = %T, want map[string]interface{}", history[0])
				}
				if entry["agent_type"] != "analyzer" {
					t.Errorf("history[0].agent_type = %v, want 'analyzer'", entry["agent_type"])
				}
			})
		})
	}
}

// TestTransientSessions_OnlyTransient_Empty verifies that when only transient sessions
// exist, the active-agents map and history are empty and all phases remain pending.
func TestTransientSessions_OnlyTransient_Empty(t *testing.T) {
	t.Parallel()

	for _, tc := range transientCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pool, svc, wfiID := setupDeriveTestEnv(t)
			tc.insert(t, pool, "s-excluded-run", wfiID, "running", "")

			if active := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{}); len(active) != 0 {
				t.Errorf("buildActiveAgentsMap with only transient = %d entries, want 0; keys: %v", len(active), mapKeys(active))
			}

			phases := svc.derivePhaseStatuses(wfiID, twoPhases)
			assertPhase(t, phases, "analyzer", "pending", "")
			assertPhase(t, phases, "builder", "pending", "")

			// Re-run for terminal history with a fresh env.
			pool2, svc2, wfiID2 := setupDeriveTestEnv(t)
			tc.insert(t, pool2, "s-excluded-term", wfiID2, "completed", "pass")
			if history := svc2.buildAgentHistory(wfiID2, map[string][]RestartDetail{}); len(history) != 0 {
				t.Errorf("buildAgentHistory with only transient = %d entries, want 0", len(history))
			}
		})
	}
}

// TestBuildActiveAgentsMap_NormalPhaseForSameAgentType_StillSurfaced verifies that a
// normal-phase session for the same agent_type as a _consult-phase session is still shown.
func TestBuildActiveAgentsMap_NormalPhaseForSameAgentType_StillSurfaced(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// _consult phase for "architect" → excluded
	insertSessionWithPhase(t, pool, "s-consult", wfiID, "architect", "_consult", "completed", "pass")
	// Normal session for "architect" in a regular phase → must appear
	insertSession(t, pool, "s-arch-normal", wfiID, "architect", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if _, ok := result["architect"]; !ok {
		t.Error("architect with a normal phase must appear in buildActiveAgentsMap")
	}
}

// TestBuildCombinedFindings_ExcludesSystemAgents verifies that context-saver and
// conflict-resolver session findings are excluded from the aggregated findings map.
func TestBuildCombinedFindings_ExcludesSystemAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-impl", wfiID, "implementor", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s-impl", "implementor", `{"k":"v"}`)

	insertSession(t, pool, "s-cs", wfiID, "context-saver", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s-cs", "context-saver", `{"to_resume":"x"}`)

	insertSession(t, pool, "s-cr", wfiID, "conflict-resolver", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s-cr", "conflict-resolver", `{"resolved":"y"}`)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 1 {
		t.Errorf("BuildCombinedFindings len = %d, want 1 (system agents excluded); keys: %v", len(combined), buildCombinedFindingsKeys(combined))
	}
	if _, ok := combined["implementor"]; !ok {
		t.Errorf("combined missing 'implementor' key; got: %v", buildCombinedFindingsKeys(combined))
	}
	if _, ok := combined["context-saver"]; ok {
		t.Error("combined must not contain 'context-saver' key")
	}
	if _, ok := combined["conflict-resolver"]; ok {
		t.Error("combined must not contain 'conflict-resolver' key")
	}
}

// TestLikeEscapeDoesNotExcludeLegitAgents verifies that the LIKE '\_%' ESCAPE '\'
// filter does not accidentally exclude legitimate agent types that contain an
// underscore in a non-leading position (e.g. "qa_verifier").
func TestLikeEscapeDoesNotExcludeLegitAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// Insert a legitimate agent with embedded underscore (non-prefix).
	// Note: it must be in the phases slice to show up in derivePhaseStatuses.
	now := "2025-01-01T00:00:00Z"
	_, err := pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES ('qa_verifier', 'test-proj', 'test-wf', '', 2, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert agent_definition qa_verifier: %v", err)
	}

	insertSession(t, pool, "s-qa", wfiID, "qa_verifier", "running", "", "")
	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "running", "", "")

	threePhases := append(twoPhases, PhaseDef{NodeID: "qa_verifier", Agent: "qa_verifier", Layer: 2})

	got := svc.derivePhaseStatuses(wfiID, threePhases)

	assertPhase(t, got, "qa_verifier", "in_progress", "")
	if _, ok := got["_finalize"]; ok {
		t.Error("_finalize must not appear as a phase key")
	}

	activeAgents := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if _, ok := activeAgents["qa_verifier"]; !ok {
		t.Error("qa_verifier (non-prefix underscore) must appear in buildActiveAgentsMap")
	}
	if _, ok := activeAgents["_finalize"]; ok {
		t.Error("_finalize must not appear in buildActiveAgentsMap")
	}
}
