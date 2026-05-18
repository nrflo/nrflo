package tools_manifest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/manifest/config"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// newTestDispatchRepo creates a test DB with migrations run and seeds a project.
// Returns a DispatchRepo and the project ID. Review functionality is disabled
// (review_items dropped in migration 114).
func newTestDispatchRepo(t *testing.T, clk clock.Clock) (*repo.DispatchRepo, string) {
	t.Helper()
	database, err := db.OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return repo.NewDispatchRepo(database, clk), "proj-1"
}

// fakeRunner records Invoke calls and returns configured output/error.
type fakeRunner struct {
	mu    sync.Mutex
	calls int
	out   []byte
	err   error
}

func (r *fakeRunner) Invoke(_ context.Context, _ string, _ []byte, _ []string, _ time.Duration) ([]byte, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.out, r.err
}

func (r *fakeRunner) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// fakeHub records Broadcast calls for assertion.
type fakeHub struct {
	mu     sync.Mutex
	events []*ws.Event
}

func (h *fakeHub) Broadcast(e *ws.Event) {
	h.mu.Lock()
	h.events = append(h.events, e)
	h.mu.Unlock()
}

func (h *fakeHub) Events() []*ws.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*ws.Event, len(h.events))
	copy(out, h.events)
	return out
}

// countEvents counts events matching the given type in hub.Events().
func countEvents(events []*ws.Event, eventType string) int {
	n := 0
	for _, e := range events {
		if e.Type == eventType {
			n++
		}
	}
	return n
}

const testManifestYAML = `
tools:
  - name: tool_no_review
    type: python_script
    description: A test tool without review
    script: tools/run.py
    input_schema:
      type: object
      properties:
        value:
          type: string
      required: [value]
  - name: tool_with_review
    type: python_script
    description: A test tool with review
    script: tools/run.py
    review: true
    input_schema:
      type: object
      properties:
        value:
          type: string
      required: [value]
`

func newTestManifest(t *testing.T) *config.Manifest {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool_manifest.yaml"), []byte(testManifestYAML), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return m
}


// env0 is a zero ToolEnv — the manifest handler does not use any env fields.
var env0 = apirun.ToolEnv{}

func TestManifestHandler_HappyPath_NoReview(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`{"result":"ok"}`)}
	hub := &fakeHub{}
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	since := clk.Now().Add(-time.Second)

	prov := New(manifest, runner, projectID, "sess-1", dispatchRepo, nil, hub, clk, nil)
	h, ok := prov.Handler("tool_no_review")
	if !ok {
		t.Fatalf("Handler: tool_no_review not found")
	}

	out, isErr, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Errorf("isErr = true, want false")
	}
	if out != `{"result":"ok"}` {
		t.Errorf("out = %q, want {\"result\":\"ok\"}", out)
	}
	if runner.Calls() != 1 {
		t.Errorf("runner calls = %d, want 1", runner.Calls())
	}

	// Dispatch row: status=success.
	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 1 || summary.Success != 1 {
		t.Errorf("dispatch Total=%d Success=%d, want 1/1", summary.Total, summary.Success)
	}

	// EventToolDispatched once; no review event (review_items dropped in migration 114).
	events := hub.Events()
	if n := countEvents(events, ws.EventToolDispatched); n != 1 {
		t.Errorf("dispatch events = %d, want 1", n)
	}
	if n := countEvents(events, ws.EventReviewCreated); n != 0 {
		t.Errorf("review events = %d, want 0", n)
	}
	for _, e := range events {
		if e.Type == ws.EventToolDispatched {
			if e.Data["status"] != model.DispatchStatusSuccess {
				t.Errorf("dispatch event status = %v, want success", e.Data["status"])
			}
			if e.Data["tool_name"] != "tool_no_review" {
				t.Errorf("dispatch event tool_name = %v, want tool_no_review", e.Data["tool_name"])
			}
		}
	}
}
