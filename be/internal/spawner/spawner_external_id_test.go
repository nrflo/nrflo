package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---- mergeExtraVars unit tests (no DB needed) ----

func TestMergeExtraVars_MergesBaseAndExtra(t *testing.T) {
	t.Parallel()
	base := map[string]string{"A": "1", "B": "2"}
	extra := map[string]string{"C": "3", "D": "4"}
	got := mergeExtraVars(base, extra)
	for k, want := range map[string]string{"A": "1", "B": "2", "C": "3", "D": "4"} {
		if got[k] != want {
			t.Errorf("mergeExtraVars[%q] = %q, want %q", k, got[k], want)
		}
	}
}

func TestMergeExtraVars_NilBase(t *testing.T) {
	t.Parallel()
	extra := map[string]string{"X": "val"}
	got := mergeExtraVars(nil, extra)
	if got["X"] != "val" {
		t.Errorf("mergeExtraVars(nil, extra)[X] = %q, want %q", got["X"], "val")
	}
}

func TestMergeExtraVars_NilExtra(t *testing.T) {
	t.Parallel()
	base := map[string]string{"A": "1"}
	got := mergeExtraVars(base, nil)
	if got["A"] != "1" {
		t.Errorf("mergeExtraVars(base, nil)[A] = %q, want %q", got["A"], "1")
	}
}

func TestMergeExtraVars_ExtraWinsOnConflict(t *testing.T) {
	t.Parallel()
	base := map[string]string{"KEY": "base-val"}
	extra := map[string]string{"KEY": "extra-val"}
	got := mergeExtraVars(base, extra)
	if got["KEY"] != "extra-val" {
		t.Errorf("mergeExtraVars conflict: [KEY] = %q, want %q (extra wins)", got["KEY"], "extra-val")
	}
}

func TestMergeExtraVars_BaseNotMutated(t *testing.T) {
	t.Parallel()
	base := map[string]string{"A": "1"}
	_ = mergeExtraVars(base, map[string]string{"A": "2"})
	if base["A"] != "1" {
		t.Errorf("mergeExtraVars mutated base map; base[A] = %q, want %q", base["A"], "1")
	}
}

// ---- fetchExternalRefs tests ----

func TestFetchExternalRefs_ReturnsWFIValues(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "EID-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET external_id=?, external_context=? WHERE id=?`,
		"ext-id-99", "ext-ctx-99", wfiID,
	); err != nil {
		t.Fatalf("update external refs: %v", err)
	}

	sp := env.newSpawner()
	gotID, gotCtx, _ := sp.fetchExternalRefs(env.project, ticketID, "test", wfiID)
	if gotID != "ext-id-99" {
		t.Errorf("fetchExternalRefs extID = %q, want %q", gotID, "ext-id-99")
	}
	if gotCtx != "ext-ctx-99" {
		t.Errorf("fetchExternalRefs extCtx = %q, want %q", gotCtx, "ext-ctx-99")
	}
}

func TestFetchExternalRefs_EmptyWhenUnset(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "EID-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	sp := env.newSpawner()
	gotID, gotCtx, _ := sp.fetchExternalRefs(env.project, ticketID, "test", wfiID)
	if gotID != "" {
		t.Errorf("fetchExternalRefs extID = %q, want empty string when external_id unset", gotID)
	}
	if gotCtx != "" {
		t.Errorf("fetchExternalRefs extCtx = %q, want empty string when external_context unset", gotCtx)
	}
}

func TestFetchExternalRefs_EmptyOnUnknownID(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	sp := env.newSpawner()
	gotID, gotCtx, _ := sp.fetchExternalRefs(env.project, "NONEXISTENT", "test", "no-such-wfi")
	if gotID != "" {
		t.Errorf("fetchExternalRefs extID = %q, want empty string on unknown wfiID", gotID)
	}
	if gotCtx != "" {
		t.Errorf("fetchExternalRefs extCtx = %q, want empty string on unknown wfiID", gotCtx)
	}
}

func TestFetchExternalRefs_EmptyOnNilPool(t *testing.T) {
	t.Parallel()
	sp := New(Config{})
	gotID, gotCtx, _ := sp.fetchExternalRefs("p", "t", "w", "wfi-1")
	if gotID != "" || gotCtx != "" {
		t.Errorf("fetchExternalRefs(nil pool) = (%q, %q), want (\"\", \"\")", gotID, gotCtx)
	}
}

// ---- prepareScriptSpawn env tests ----

// insertScriptWFI inserts a workflow + workflow_instance row into env.database for
// prepareScriptSpawn env tests.
func insertScriptWFI(t *testing.T, env *scriptSpawnEnv, extID, extCtx string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// workflow row (ignore duplicate-insert errors — idempotent)
	_, _ = env.database.Exec(
		`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES (?, 'test', 'test', 'ticket', ?, ?)`,
		env.projectID, now, now,
	)
	wfiID := uuid.New().String()
	if _, err := env.database.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, external_id, external_context, created_at, updated_at)
		 VALUES (?, ?, '', 'test', 'ticket', 'active', ?, ?, ?, ?)`,
		wfiID, env.projectID, extID, extCtx, now, now,
	); err != nil {
		t.Fatalf("insertScriptWFI: %v", err)
	}
	return wfiID
}

func TestPrepareScriptSpawn_ExternalIDInEnv_WhenSet(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	wfiID := insertScriptWFI(t, env, "extid-val", "extctx-val")

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", wfiID, "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if !projEnvContains(prep.opts.Env, "NRF_EXTERNAL_ID=extid-val") {
		t.Errorf("opts.Env missing NRF_EXTERNAL_ID=extid-val; env=%v", prep.opts.Env)
	}
	if !projEnvContains(prep.opts.Env, "NRF_EXTERNAL_CONTEXT=extctx-val") {
		t.Errorf("opts.Env missing NRF_EXTERNAL_CONTEXT=extctx-val; env=%v", prep.opts.Env)
	}
}

func TestPrepareScriptSpawn_ExternalIDInEnv_EmptyWhenUnset(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	wfiID := insertScriptWFI(t, env, "", "")

	agentDef := makeMinimalAgentDef(env.scriptID)
	_, prep, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", wfiID, "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if !projEnvHasKey(prep.opts.Env, "NRF_EXTERNAL_ID") {
		t.Errorf("opts.Env must contain NRF_EXTERNAL_ID key (even when empty); env=%v", prep.opts.Env)
	}
	if !projEnvHasKey(prep.opts.Env, "NRF_EXTERNAL_CONTEXT") {
		t.Errorf("opts.Env must contain NRF_EXTERNAL_CONTEXT key (even when empty); env=%v", prep.opts.Env)
	}
	// Confirm both are present-but-empty
	for _, e := range prep.opts.Env {
		if e == "NRF_EXTERNAL_ID=" {
			goto checkCtx
		}
	}
	t.Errorf("NRF_EXTERNAL_ID not present-but-empty; env=%v", prep.opts.Env)
checkCtx:
	for _, e := range prep.opts.Env {
		if e == "NRF_EXTERNAL_CONTEXT=" {
			return
		}
	}
	t.Errorf("NRF_EXTERNAL_CONTEXT not present-but-empty; env=%v", prep.opts.Env)
}

// TestLoadTemplate_ExternalIDExpansion verifies ${EXTERNAL_ID} and
// ${EXTERNAL_CONTEXT} expand from the WFI when passed via merged ExtraVars.
func TestLoadTemplate_ExternalIDExpansion(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "EID-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET external_id=?, external_context=? WHERE id=?`,
		"jira-123", "context-payload", wfiID,
	); err != nil {
		t.Fatalf("update external refs: %v", err)
	}

	createAgentDef(t, env, "analyzer", "ID=${EXTERNAL_ID} CTX=${EXTERNAL_CONTEXT}")

	sp := env.newSpawner()
	extID, extCtx, _ := sp.fetchExternalRefs(env.project, ticketID, "test", wfiID)
	extraVars := mergeExtraVars(nil, map[string]string{"EXTERNAL_ID": extID, "EXTERNAL_CONTEXT": extCtx})

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "s", "test", "claude:sonnet-5", "", wfiID, extraVars, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "ID=jira-123") {
		t.Errorf("EXTERNAL_ID not expanded; got: %s", result)
	}
	if !strings.Contains(result, "CTX=context-payload") {
		t.Errorf("EXTERNAL_CONTEXT not expanded; got: %s", result)
	}
}

func TestLoadTemplate_ExternalID_EmptyWhenUnset(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "EID-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDef(t, env, "analyzer", "ID=${EXTERNAL_ID} CTX=${EXTERNAL_CONTEXT}")

	sp := env.newSpawner()
	extID, extCtx, _ := sp.fetchExternalRefs(env.project, ticketID, "test", wfiID)
	extraVars := mergeExtraVars(nil, map[string]string{"EXTERNAL_ID": extID, "EXTERNAL_CONTEXT": extCtx})

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "s", "test", "claude:sonnet-5", "", wfiID, extraVars, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	// Both placeholders expand to empty strings — pattern becomes "ID= CTX=".
	if !strings.Contains(result, "ID=") {
		t.Errorf("EXTERNAL_ID placeholder not processed; got: %s", result)
	}
	if strings.Contains(result, "${EXTERNAL_ID}") {
		t.Errorf("${EXTERNAL_ID} literal still present (not expanded); got: %s", result)
	}
	if strings.Contains(result, "${EXTERNAL_CONTEXT}") {
		t.Errorf("${EXTERNAL_CONTEXT} literal still present (not expanded); got: %s", result)
	}
}
