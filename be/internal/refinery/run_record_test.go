package refinery

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// stubBuildProviderErr swaps buildProvider to always fail with err, mirroring
// stubBuildProvider (fold_test.go) but for the build-failure path.
func stubBuildProviderErr(t *testing.T, err error) {
	t.Helper()
	orig := buildProvider
	buildProvider = func(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) {
		return nil, err
	}
	t.Cleanup(func() { buildProvider = orig })
}

func queryRefineryRuns(t *testing.T, pool *db.Pool) []refineryRunRow {
	t.Helper()
	rows, err := pool.Query(`SELECT session_id, workflow_instance_id, node_id, provider, model, prompt_tokens, output_tokens, status, error FROM refinery_runs ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query refinery_runs: %v", err)
	}
	defer rows.Close()
	var out []refineryRunRow
	for rows.Next() {
		var r refineryRunRow
		if err := rows.Scan(&r.sessionID, &r.wfiID, &r.nodeID, &r.provider, &r.model, &r.promptTokens, &r.outputTokens, &r.status, &r.errMsg); err != nil {
			t.Fatalf("scan refinery_runs row: %v", err)
		}
		out = append(out, r)
	}
	return out
}

type refineryRunRow struct {
	sessionID    string
	wfiID        string
	nodeID       string
	provider     string
	model        string
	promptTokens int
	outputTokens int
	status       string
	errMsg       string
}

// TestRecordFoldRun_SuccessWritesOkRow verifies a successful console fold
// writes one refinery_runs row with status='ok', non-zero prompt/output
// tokens, provider+model populated, and session_id set.
func TestRecordFoldRun_SuccessWritesOkRow(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-run-ok", "proj-run-ok"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mock.Script{Final: provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "digest v1"}},
		Usage:      provider.Usage{InputTokens: 42, OutputTokens: 7},
	}}))

	mgr.fold(context.Background(), sessionID, projectID, []string{`{"type":"findings.updated"}`})

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.sessionID != sessionID {
		t.Errorf("session_id = %q, want %q", r.sessionID, sessionID)
	}
	if r.status != "ok" {
		t.Errorf("status = %q, want ok", r.status)
	}
	if r.promptTokens == 0 || r.outputTokens == 0 {
		t.Errorf("tokens = %d/%d, want both non-zero", r.promptTokens, r.outputTokens)
	}
	if r.provider == "" || r.model == "" {
		t.Errorf("provider/model = %q/%q, want both populated", r.provider, r.model)
	}
}

// TestRecordFoldRun_FailureWritesFailedRowAndBroadcasts reproduces a
// "no anthropic API key" build failure: asserts a status='failed' row whose
// error contains the message and whose provider matches the model row's
// provider, and that refinery.fold_failed reaches a stub broadcaster.
func TestRecordFoldRun_FailureWritesFailedRowAndBroadcasts(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-run-fail", "proj-run-fail"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProviderErr(t, errors.New("no anthropic API key"))

	var mu sync.Mutex
	var events []*ws.Event
	mgr.SetBroadcaster(func(ev *ws.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	})

	mgr.fold(context.Background(), sessionID, projectID, []string{"event"})

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.status != "failed" {
		t.Errorf("status = %q, want failed", r.status)
	}
	if !strings.Contains(r.errMsg, "no anthropic API key") {
		t.Errorf("error = %q, want it to contain %q", r.errMsg, "no anthropic API key")
	}
	if r.provider == "" {
		t.Error("provider = \"\", want the model row's provider populated even on build failure")
	}

	// Guard: no digest row was written on failure (existing invariant).
	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get digest: %v", err)
	}
	if d != nil {
		t.Errorf("digest after failed fold = %+v, want nil (no digest written on failure)", d)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range events {
		if ev.Type == ws.EventRefineryFoldFailed {
			found = true
			if ev.Data["session_id"] != sessionID {
				t.Errorf("event session_id = %v, want %q", ev.Data["session_id"], sessionID)
			}
		}
	}
	if !found {
		t.Error("expected a refinery.fold_failed broadcast, got none")
	}
}

// TestRecordFoldRun_AutonomousPathCarriesWorkflowAndNode verifies the
// autonomous fold path (foldAutonomous) writes a refinery_runs row carrying
// workflow_instance_id + node_id (not session_id-only, as the console path
// does).
func TestRecordFoldRun_AutonomousPathCarriesWorkflowAndNode(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-run-auto", "proj-run-auto"
	wfiID, nodeID := "wfi-run-auto", "node-run-auto"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "work one")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mock.Script{Final: provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "autonomous digest v1"}},
		Usage:      provider.Usage{InputTokens: 5, OutputTokens: 3},
	}}))

	as := mgr.newAutonomousSession(wfiID, nodeID, "")
	mgr.foldAutonomous(context.Background(), as, sessionID, projectID)

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.wfiID != wfiID || r.nodeID != nodeID {
		t.Errorf("wfiID/nodeID = %q/%q, want %q/%q", r.wfiID, r.nodeID, wfiID, nodeID)
	}
	if r.status != "ok" {
		t.Errorf("status = %q, want ok", r.status)
	}
}
