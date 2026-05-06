package spawner

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// createAgentDefWithLayer inserts a project-scoped agent definition for the "test"
// workflow at the given layer.
func createAgentDefWithLayer(t *testing.T, env *spawnerTestEnv, agentID, prompt string, layer int) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("createAgentDefWithLayer: open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:         agentID,
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "sonnet",
		Timeout:    3600,
		Prompt:     prompt,
		Layer:      layer,
	})
	if err != nil {
		t.Fatalf("createAgentDefWithLayer(%s, layer=%d): %v", agentID, layer, err)
	}
}

// insertCompletedLayerSession inserts a completed agent_sessions row for layer findings tests.
func insertCompletedLayerSession(t *testing.T, pool *db.Pool, sessionID, projectID, ticketID, wfiID, agentType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, result_reason, pid, findings, context_left, ancestor_session_id,
			spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
		sessionID, projectID, ticketID, wfiID, agentType, agentType, now, now, now, now)
	if err != nil {
		t.Fatalf("insertCompletedLayerSession(%s): %v", sessionID, err)
	}
}

// setLayerSessionFindings updates the findings JSON on a session row.
func setLayerSessionFindings(t *testing.T, pool *db.Pool, sessionID, findingsJSON string) {
	t.Helper()
	_, err := pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`, findingsJSON, sessionID)
	if err != nil {
		t.Fatalf("setLayerSessionFindings(%s): %v", sessionID, err)
	}
}

// setupLayer1ScraperFixture creates the standard 3-sibling layer-1 fixture:
//   - scrape-g2:     session with single key {"review_count": 5}
//   - scrape-news:   no session row → "  _No findings_" in output
//   - scrape-reddit: session with rich findings {"posts":["post-a","post-b"],"source":"reddit"}
//
// Returns (wfiID, ticketID).
func setupLayer1ScraperFixture(t *testing.T, env *spawnerTestEnv) (wfiID, ticketID string) {
	t.Helper()

	ticketID = "LF-" + uuid.New().String()[:6]
	wfiID = env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "scrape-g2", "Scrape G2 reviews", 1)
	createAgentDefWithLayer(t, env, "scrape-news", "Scrape news articles", 1)
	createAgentDefWithLayer(t, env, "scrape-reddit", "Scrape reddit posts", 1)

	g2SessID := "sess-g2-" + uuid.New().String()[:6]
	insertCompletedLayerSession(t, env.pool, g2SessID, env.project, ticketID, wfiID, "scrape-g2")
	setLayerSessionFindings(t, env.pool, g2SessID, `{"review_count": 5}`)

	redditSessID := "sess-reddit-" + uuid.New().String()[:6]
	insertCompletedLayerSession(t, env.pool, redditSessID, env.project, ticketID, wfiID, "scrape-reddit")
	setLayerSessionFindings(t, env.pool, redditSessID, `{"posts": ["post-a", "post-b"], "source": "reddit"}`)

	// scrape-news intentionally has no session row → nil in layer map → "  _No findings_"
	return wfiID, ticketID
}

// TestLoadTemplate_PriorLayerFindings_RosterSortedWithMissing verifies that
// #{PRIOR_LAYER_FINDINGS} in a layer-2 agent's prompt expands to the full
// layer-1 sibling roster: all three agent types sorted alphabetically, rich
// findings printed in full, agents without sessions rendered as two-space-
// indented "_No findings_".
func TestLoadTemplate_PriorLayerFindings_RosterSortedWithMissing(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	wfiID, ticketID := setupLayer1ScraperFixture(t, env)

	createAgentDefWithLayer(t, env, "merger", "## Prior Scrapers\n#{PRIOR_LAYER_FINDINGS}\n\nMerge all.", 2)

	sp := env.newSpawner()
	result, _, err := sp.loadTemplate("merger", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 2)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if strings.Contains(result, "#{PRIOR_LAYER_FINDINGS}") {
		t.Error("#{PRIOR_LAYER_FINDINGS} was not consumed")
	}

	// All three agent types must appear as headers
	for _, header := range []string{"scrape-g2:", "scrape-news:", "scrape-reddit:"} {
		if !strings.Contains(result, header) {
			t.Errorf("expected agent header %q in result:\n%s", header, result)
		}
	}

	// Alphabetical order: scrape-g2 < scrape-news < scrape-reddit
	g2Idx := strings.Index(result, "scrape-g2:")
	newsIdx := strings.Index(result, "scrape-news:")
	redditIdx := strings.Index(result, "scrape-reddit:")
	if g2Idx > newsIdx || newsIdx > redditIdx {
		t.Errorf("expected alphabetical order g2(%d) < news(%d) < reddit(%d)\nResult:\n%s",
			g2Idx, newsIdx, redditIdx, result)
	}

	// scrape-g2 has a single numeric key — two-space indented
	if !strings.Contains(result, "  review_count: 5") {
		t.Errorf("expected '  review_count: 5' for scrape-g2:\n%s", result)
	}

	// scrape-news has no session — two-space indented _No findings_
	if !strings.Contains(result, "  _No findings_") {
		t.Errorf("expected '  _No findings_' for scrape-news:\n%s", result)
	}

	// scrape-reddit has rich findings — source string and posts list
	if !strings.Contains(result, "  source: reddit") {
		t.Errorf("expected '  source: reddit' in scrape-reddit:\n%s", result)
	}
	if !strings.Contains(result, "post-a") || !strings.Contains(result, "post-b") {
		t.Errorf("expected posts list items in scrape-reddit:\n%s", result)
	}
}

// TestLoadTemplate_LayerFindingsExplicit_IdenticalToPriorLayer verifies that
// #{LAYER_FINDINGS:1} from a layer-2 agent renders identically to
// #{PRIOR_LAYER_FINDINGS} from the same layer.
func TestLoadTemplate_LayerFindingsExplicit_IdenticalToPriorLayer(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	wfiID, ticketID := setupLayer1ScraperFixture(t, env)

	createAgentDefWithLayer(t, env, "merger-prior", "#{PRIOR_LAYER_FINDINGS}", 2)
	createAgentDefWithLayer(t, env, "merger-explicit", "#{LAYER_FINDINGS:1}", 2)

	sp := env.newSpawner()

	prior, _, err := sp.loadTemplate("merger-prior", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 2)
	if err != nil {
		t.Fatalf("loadTemplate(merger-prior) failed: %v", err)
	}

	explicit, _, err := sp.loadTemplate("merger-explicit", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 2)
	if err != nil {
		t.Fatalf("loadTemplate(merger-explicit) failed: %v", err)
	}

	if prior != explicit {
		t.Errorf("#{PRIOR_LAYER_FINDINGS} and #{LAYER_FINDINGS:1} produced different results.\nPRIOR:\n%q\nEXPLICIT:\n%q",
			prior, explicit)
	}
}

// TestLoadTemplate_PriorLayerFindings_Layer0_NoPriorLayer verifies that
// #{PRIOR_LAYER_FINDINGS} in a layer-0 agent's prompt expands to "_No prior layer_"
// without error and without calling the FindingsService.
func TestLoadTemplate_PriorLayerFindings_Layer0_NoPriorLayer(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "L0-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "l0-agent", "Context: #{PRIOR_LAYER_FINDINGS}\n\nRun analysis.", 0)

	sp := env.newSpawner()
	result, _, err := sp.loadTemplate("l0-agent", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed unexpectedly: %v", err)
	}

	if strings.Contains(result, "#{PRIOR_LAYER_FINDINGS}") {
		t.Error("#{PRIOR_LAYER_FINDINGS} was not expanded")
	}
	if !strings.Contains(result, "_No prior layer_") {
		t.Errorf("expected '_No prior layer_' for layer-0 agent; got:\n%s", result)
	}
}
