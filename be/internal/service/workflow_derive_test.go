package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// setupDeriveTestEnv creates a minimal DB stack for testing derivePhaseStatuses
// and deriveCurrentPhase. Returns pool, service, and the wfiID to use.
func setupDeriveTestEnv(t *testing.T) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "derive_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "test-proj"
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	phasesJSON := `[{"agent":"analyzer","layer":0},{"agent":"builder","layer":1}]`
	if _, err = pool.Exec(
		`INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at) VALUES (?, ?, '', ?, 'ticket', ?, ?)`,
		"test-wf", projectID, phasesJSON, now, now); err != nil {
		t.Fatalf("workflow insert: %v", err)
	}

	wfiID := "wfi-test"
	if _, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', 'test-wf', 'ticket', 'active', '{}', 0, ?, ?)`,
		wfiID, projectID, now, now); err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())
	return pool, svc, wfiID
}

// insertSession inserts an agent_session row directly for testing.
// createdAt controls the ordering used by derivePhaseStatuses (latest wins).
func insertSession(t *testing.T, pool *db.Pool, id, wfiID, agentType, status, result, createdAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if createdAt == "" {
		createdAt = now
	}
	var resultVal interface{}
	if result != "" {
		resultVal = result
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, result_reason, pid, findings, context_left, ancestor_session_id,
			spawn_command, prompt_context, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		id, wfiID, agentType, agentType, status, resultVal, createdAt, createdAt, now)
	if err != nil {
		t.Fatalf("failed to insert session %s: %v", id, err)
	}
}

// twoPhases returns a standard 2-phase definition for tests.
var twoPhases = []PhaseDef{
	{ID: "analyzer", Agent: "analyzer", Layer: 0},
	{ID: "builder", Agent: "builder", Layer: 1},
}

func TestDerivePhaseStatuses(t *testing.T) {
	type wantPhase struct {
		status string
		result string
	}
	tests := []struct {
		name         string
		setupFn      func(pool *db.Pool, wfiID string)
		wantAnalyzer wantPhase
		wantBuilder  wantPhase
	}{
		{
			name:         "no sessions → all pending",
			setupFn:      func(pool *db.Pool, wfiID string) {},
			wantAnalyzer: wantPhase{"pending", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "running session → in_progress",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "running", "", "")
			},
			wantAnalyzer: wantPhase{"in_progress", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "user_interactive session → in_progress",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "user_interactive", "", "")
			},
			wantAnalyzer: wantPhase{"in_progress", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "completed session with pass → completed/pass",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
			},
			wantAnalyzer: wantPhase{"completed", "pass"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "completed session with fail result → completed/fail",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "fail", "")
			},
			wantAnalyzer: wantPhase{"completed", "fail"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "failed session → completed/fail",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "failed", "fail", "")
			},
			wantAnalyzer: wantPhase{"completed", "fail"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "timeout session → completed/timeout",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "timeout", "timeout", "")
			},
			wantAnalyzer: wantPhase{"completed", "timeout"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "continued session excluded → pending",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "continued", "", "")
			},
			wantAnalyzer: wantPhase{"pending", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "callback session excluded → pending",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "callback", "callback", "")
			},
			wantAnalyzer: wantPhase{"pending", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "parallel agents both running → both in_progress",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "running", "", "")
				insertSession(t, pool, "s2", wfiID, "builder", "running", "", "")
			},
			wantAnalyzer: wantPhase{"in_progress", ""},
			wantBuilder:  wantPhase{"in_progress", ""},
		},
		{
			name: "builder completed → analyzer inferred skipped",
			setupFn: func(pool *db.Pool, wfiID string) {
				// Only builder has a session; analyzer has none but layer 1 > layer 0
				insertSession(t, pool, "s1", wfiID, "builder", "completed", "pass", "")
			},
			wantAnalyzer: wantPhase{"completed", "skipped"},
			wantBuilder:  wantPhase{"completed", "pass"},
		},
		{
			name: "latest session wins (continued then running)",
			setupFn: func(pool *db.Pool, wfiID string) {
				t1 := "2025-01-01T00:00:00Z"
				t2 := "2025-01-01T00:00:01Z"
				insertSession(t, pool, "s-old", wfiID, "analyzer", "continued", "", t1)
				insertSession(t, pool, "s-new", wfiID, "analyzer", "running", "", t2)
			},
			// continued excluded; running session picked
			wantAnalyzer: wantPhase{"in_progress", ""},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "latest non-excluded session wins (two completed)",
			setupFn: func(pool *db.Pool, wfiID string) {
				t1 := "2025-01-01T00:00:00Z"
				t2 := "2025-01-01T00:00:01Z"
				insertSession(t, pool, "s-old", wfiID, "analyzer", "completed", "fail", t1)
				insertSession(t, pool, "s-new", wfiID, "analyzer", "completed", "pass", t2)
			},
			// DESC order: s-new first → pass
			wantAnalyzer: wantPhase{"completed", "pass"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "interactive_completed → completed/pass",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "interactive_completed", "pass", "")
			},
			wantAnalyzer: wantPhase{"completed", "pass"},
			wantBuilder:  wantPhase{"pending", ""},
		},
		{
			name: "project_completed status → completed/pass",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "project_completed", "pass", "")
			},
			wantAnalyzer: wantPhase{"completed", "pass"},
			wantBuilder:  wantPhase{"pending", ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			tc.setupFn(pool, wfiID)

			got := svc.derivePhaseStatuses(wfiID, twoPhases)

			assertPhase(t, got, "analyzer", tc.wantAnalyzer.status, tc.wantAnalyzer.result)
			assertPhase(t, got, "builder", tc.wantBuilder.status, tc.wantBuilder.result)
		})
	}
}

// assertPhase checks a phase in the derived map.
func assertPhase(t *testing.T, phases map[string]model.PhaseStatus, name, wantStatus, wantResult string) {
	t.Helper()
	ps, ok := phases[name]
	if !ok {
		t.Errorf("phase %q not found in derived map", name)
		return
	}
	if ps.Status != wantStatus {
		t.Errorf("phase %q status = %q, want %q", name, ps.Status, wantStatus)
	}
	if ps.Result != wantResult {
		t.Errorf("phase %q result = %q, want %q", name, ps.Result, wantResult)
	}
}

func TestDerivePhaseStatuses_UnknownPhaseIgnored(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// Insert session for an agent_type not in the phases slice
	insertSession(t, pool, "s1", wfiID, "unknown-agent", "completed", "pass", "")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	// Unknown agent session doesn't affect known phases
	assertPhase(t, got, "analyzer", "pending", "")
	assertPhase(t, got, "builder", "pending", "")
}

func TestDerivePhaseStatuses_EmptyPhasesSlice(t *testing.T) {
	_, svc, wfiID := setupDeriveTestEnv(t)

	got := svc.derivePhaseStatuses(wfiID, []PhaseDef{})

	if len(got) != 0 {
		t.Errorf("expected empty map for empty phases slice, got %d entries", len(got))
	}
}

func TestDeriveCurrentPhase(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(pool *db.Pool, wfiID string)
		want    string
	}{
		{
			name:    "no sessions → empty string",
			setupFn: func(pool *db.Pool, wfiID string) {},
			want:    "",
		},
		{
			name: "running session → returns phase",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "running", "", "")
			},
			want: "analyzer",
		},
		{
			name: "user_interactive session → returns phase",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "builder", "user_interactive", "", "")
			},
			want: "builder",
		},
		{
			name: "completed session → empty string (not running)",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
			},
			want: "",
		},
		{
			name: "failed session → empty string",
			setupFn: func(pool *db.Pool, wfiID string) {
				insertSession(t, pool, "s1", wfiID, "analyzer", "failed", "fail", "")
			},
			want: "",
		},
		{
			name: "latest running session wins",
			setupFn: func(pool *db.Pool, wfiID string) {
				t1 := "2025-01-01T00:00:00Z"
				t2 := "2025-01-01T00:00:01Z"
				insertSession(t, pool, "s1", wfiID, "analyzer", "running", "", t1)
				insertSession(t, pool, "s2", wfiID, "builder", "running", "", t2)
			},
			// Latest (t2) is builder
			want: "builder",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			tc.setupFn(pool, wfiID)

			got := svc.deriveCurrentPhase(wfiID)
			if got != tc.want {
				t.Errorf("deriveCurrentPhase() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDerivePhaseStatuses_ThreeLayerSkipInference(t *testing.T) {
	pool, svc, _ := setupDeriveTestEnv(t)

	// Add extra workflow with 3 layers
	now := time.Now().UTC().Format(time.RFC3339Nano)
	phasesJSON3 := `[{"agent":"p1","layer":0},{"agent":"p2","layer":1},{"agent":"p3","layer":2}]`
	wfID := "test-wf-3"
	_, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at) VALUES (?, 'test-proj', '', ?, 'ticket', ?, ?)`,
		wfID, phasesJSON3, now, now)
	if err != nil {
		t.Fatalf("workflow insert: %v", err)
	}

	wfiID2 := "wfi-test-3"
	_, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, 'test-proj', '', ?, 'ticket', 'active', '{}', 0, ?, ?)`,
		wfiID2, wfID, now, now)
	if err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	// Only p3 (layer 2) has a session; p1 and p2 should be inferred as skipped
	insertSession(t, pool, "s1", wfiID2, "p3", "completed", "pass", "")

	phases3 := []PhaseDef{
		{ID: "p1", Agent: "p1", Layer: 0},
		{ID: "p2", Agent: "p2", Layer: 1},
		{ID: "p3", Agent: "p3", Layer: 2},
	}
	got := svc.derivePhaseStatuses(wfiID2, phases3)

	assertPhase(t, got, "p1", "completed", "skipped")
	assertPhase(t, got, "p2", "completed", "skipped")
	assertPhase(t, got, "p3", "completed", "pass")
}

// TestDerivePhaseStatuses_NoSkipWhenNoLaterLayers verifies that a phase with no session
// is NOT inferred as skipped when no later layers have sessions.
func TestDerivePhaseStatuses_NoSkipWhenNoLaterLayers(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// Only analyzer (layer 0) has a session; builder (layer 1) has none.
	// builder should remain pending, NOT skipped.
	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	assertPhase(t, got, "analyzer", "completed", "pass")
	assertPhase(t, got, "builder", "pending", "") // NOT skipped
}

// TestDerivePhaseStatuses_SessionForDifferentWFI verifies isolation between
// workflow instances — sessions from another instance don't affect results.
func TestDerivePhaseStatuses_SessionForDifferentWFI(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// Insert a second workflow def so the FK constraint passes
	now := time.Now().UTC().Format(time.RFC3339Nano)
	phasesJSON := `[{"agent":"analyzer","layer":0},{"agent":"builder","layer":1}]`
	_, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at) VALUES (?, 'test-proj', '', ?, 'ticket', ?, ?)`,
		"test-wf-other", phasesJSON, now, now)
	if err != nil {
		t.Fatalf("workflow insert: %v", err)
	}

	// Insert session for a DIFFERENT workflow instance ID
	_, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, 'test-proj', '', 'test-wf-other', 'ticket', 'active', '{}', 0, ?, ?)`,
		"other-wfi", now, now)
	if err != nil {
		t.Fatalf("other wfi insert: %v", err)
	}
	insertSession(t, pool, "s-other", "other-wfi", "analyzer", "completed", "pass", "")

	// Our wfiID has no sessions
	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	assertPhase(t, got, "analyzer", "pending", "")
	assertPhase(t, got, "builder", "pending", "")
}

// BenchmarkDerivePhaseStatuses measures the performance of phase derivation.
func BenchmarkDerivePhaseStatuses(b *testing.B) {
	pool, svc, wfiID := setupDeriveTestEnvBench(b)

	// Insert 5 sessions
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 5; i++ {
		pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
				status, result, result_reason, pid, findings, context_left, ancestor_session_id,
				spawn_command, prompt_context, restart_count, started_at, ended_at, created_at, updated_at)
			VALUES (?, 'test-proj', '', ?, ?, ?, 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
			fmt.Sprintf("sess-%d", i), wfiID, fmt.Sprintf("agent-%d", i), fmt.Sprintf("agent-%d", i),
			now, now, now)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.derivePhaseStatuses(wfiID, twoPhases)
	}
}

func setupDeriveTestEnvBench(b *testing.B) (*db.Pool, *WorkflowService, string) {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "bench.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		b.Fatalf("failed to create pool: %v", err)
	}
	b.Cleanup(func() { pool.Close() })

	projectID := "test-proj"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`, projectID, now, now)
	pool.Exec(`INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at) VALUES ('test-wf', ?, '', '[{"agent":"analyzer","layer":0},{"agent":"builder","layer":1}]', 'ticket', ?, ?)`, projectID, now, now)
	wfiID := "bench-wfi"
	pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at) VALUES (?, ?, '', 'test-wf', 'ticket', 'active', '{}', 0, ?, ?)`, wfiID, projectID, now, now)

	svc := NewWorkflowService(pool, clock.Real())
	return pool, svc, wfiID
}
