package spawner

// Tests for stepwiseResumeData's ${previous_data} content, wired through
// fetchPreviousDataAndReason's stepwise short-circuit
// (template_findings_prev.go).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// twoStepResumeSteps returns step-one (completed, requires "summary" +
// "changes") and step-two (current, distinctive instruction text used to
// assert single-occurrence rendering).
func twoStepResumeSteps() []model.StepDefinition {
	return []model.StepDefinition{
		{
			StepID: "step-one", Title: "First", Instruction: "First instruction.",
			RequiredFindings: []model.RequiredFinding{
				{Key: "summary", Schema: model.FindingSchemaNonemptyText},
				{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange},
			},
		},
		{StepID: "step-two", Title: "Second", Instruction: "Only-once current instruction text."},
	}
}

// setupResumeEnv builds a stepwise def with step-one completed (with
// findings) and step-two current, plus a continued session on the same node
// carrying both the required findings and a to_resume finding that must NOT
// be used. Returns (env, wfiID, ticketID).
func setupResumeEnv(t *testing.T) (*spawnerTestEnv, string, string) {
	t.Helper()
	env := newSpawnerTestEnv(t)
	ticketID := "SR-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	var root string
	if err := env.pool.QueryRow(`SELECT root_path FROM projects WHERE id = ?`, env.project).Scan(&root); err != nil {
		t.Fatalf("read project root_path: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "real.go"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write real.go: %v", err)
	}

	createStepwiseAgentDef(t, env, "analyzer", twoStepResumeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	if _, err := env.pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		`[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`, wfiID, "analyzer"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	createContinuedSessionInEnv(t, env, ticketID, wfiID, "analyzer", "claude:sonnet-5", "analyzer", "low_context",
		map[string]interface{}{
			"to_resume": "SHOULD NOT APPEAR ANYWHERE",
			"summary":   "did the first thing",
			"changes": []map[string]string{
				{"path": "pkg/real.go", "change": "added"},
				{"path": "definitely/bogus/path.go", "change": "added"},
			},
		})

	return env, wfiID, ticketID
}

// TestLoadTemplate_StepwiseRelaunch_PreviousDataFromCursorEvidence covers the
// full relaunch previous_data contract: completed step's required-findings
// keys/values + resolved/unresolved path refs present, the to_resume finding
// on the same session never used, and the current step's instruction
// appearing exactly once in the whole prompt.
func TestLoadTemplate_StepwiseRelaunch_PreviousDataFromCursorEvidence(t *testing.T) {
	t.Parallel()
	env, wfiID, ticketID := setupResumeEnv(t)
	sp := env.newSpawner()

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "## Completed Steps (verified)") {
		t.Error("expected the completed-steps evidence header")
	}
	if !strings.Contains(result, "did the first thing") {
		t.Errorf("expected the completed step's finding value present, got: %s", result)
	}
	if !strings.Contains(result, "pkg/real.go") {
		t.Error("expected the resolved path present")
	}
	if !strings.Contains(result, "definitely/bogus/path.go") {
		t.Error("expected the unresolved path present (never dropped)")
	}
	if strings.Contains(result, "SHOULD NOT APPEAR ANYWHERE") {
		t.Error("to_resume finding must never be used for a stepwise def")
	}
	if got := strings.Count(result, "Only-once current instruction text."); got != 1 {
		t.Errorf("current step instruction occurrence count = %d, want exactly 1", got)
	}
}

// TestLoadTemplate_StepwiseRelaunch_FreshDigestUnderNarrativeLabel verifies a
// fresh refinery slot digest (updated_at >= previous session's started_at)
// renders under handoff's non-authoritative "## Narrative Summary" label.
func TestLoadTemplate_StepwiseRelaunch_FreshDigestUnderNarrativeLabel(t *testing.T) {
	t.Parallel()
	env, wfiID, ticketID := setupResumeEnv(t)
	sp := env.newSpawner()

	digestRepo := repo.NewRefineryDigestRepo(env.pool, clock.Real())
	if _, err := digestRepo.UpsertSlot(wfiID, "analyzer", env.project, "FRESH DIGEST TEXT"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "## Narrative Summary") {
		t.Error("expected the non-authoritative narrative label present for a fresh digest")
	}
	if !strings.Contains(result, "FRESH DIGEST TEXT") {
		t.Errorf("expected the digest text present, got: %s", result)
	}
	narrIdx := strings.Index(result, "## Narrative Summary")
	evidIdx := strings.Index(result, "## Completed Steps (verified)")
	if evidIdx == -1 || narrIdx == -1 || narrIdx < evidIdx {
		t.Errorf("expected the narrative section after the completed-steps evidence, narrIdx=%d evidIdx=%d", narrIdx, evidIdx)
	}
}

// TestLoadTemplate_StepwiseRelaunch_StaleOrEmptyDigest_NoNarrativeSection
// verifies a stale digest (folded before the previous session's start) and
// no digest at all both produce previous_data with no narrative section.
func TestLoadTemplate_StepwiseRelaunch_StaleOrEmptyDigest_NoNarrativeSection(t *testing.T) {
	t.Parallel()

	t.Run("stale digest", func(t *testing.T) {
		t.Parallel()
		env, wfiID, ticketID := setupResumeEnv(t)
		sp := env.newSpawner()

		// Seed the digest with a clock far in the past — the continued
		// session (created inside setupResumeEnv, started_at ~= now) is
		// necessarily later, so this digest is stale.
		staleClk := clock.NewTest(time.Now().Add(-24 * time.Hour))
		digestRepo := repo.NewRefineryDigestRepo(env.pool, staleClk)
		if _, err := digestRepo.UpsertSlot(wfiID, "analyzer", env.project, "STALE DIGEST TEXT"); err != nil {
			t.Fatalf("UpsertSlot: %v", err)
		}

		result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
			"claude:sonnet-5", "analyzer", wfiID, nil, 0)
		if err != nil {
			t.Fatalf("loadTemplate failed: %v", err)
		}
		if strings.Contains(result, "## Narrative Summary") || strings.Contains(result, "STALE DIGEST TEXT") {
			t.Errorf("stale digest must not appear under the narrative label, got: %s", result)
		}
	})

	t.Run("no digest", func(t *testing.T) {
		t.Parallel()
		env, wfiID, ticketID := setupResumeEnv(t)
		sp := env.newSpawner()

		result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
			"claude:sonnet-5", "analyzer", wfiID, nil, 0)
		if err != nil {
			t.Fatalf("loadTemplate failed: %v", err)
		}
		if strings.Contains(result, "## Narrative Summary") {
			t.Error("no digest exists — narrative section must be absent")
		}
	})
}
