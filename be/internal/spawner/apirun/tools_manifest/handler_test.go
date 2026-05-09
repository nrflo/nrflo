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

func newTestRepos(t *testing.T, clk clock.Clock) (*repo.DispatchRepo, *repo.ReviewRepo, string) {
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
	return repo.NewDispatchRepo(database, clk),
		repo.NewReviewRepo(database, clk),
		"proj-1"
}

// env0 is a zero ToolEnv — the manifest handler does not use any env fields.
var env0 = apirun.ToolEnv{}

func TestManifestHandler_HappyPath_NoReview(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`{"result":"ok"}`)}
	hub := &fakeHub{}
	dispatchRepo, reviewRepo, projectID := newTestRepos(t, clk)
	since := clk.Now().Add(-time.Second)

	prov := New(manifest, runner, projectID, "sess-1", dispatchRepo, reviewRepo, hub, clk, nil)
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

	// No review row.
	reviews, rErr := reviewRepo.List(projectID, "", 10, 0)
	if rErr != nil {
		t.Fatalf("List reviews: %v", rErr)
	}
	if len(reviews) != 0 {
		t.Errorf("review items = %d, want 0 (review:false)", len(reviews))
	}

	// EventToolDispatched once; no review event.
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

func TestManifestHandler_HappyPath_WithReview(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`{"result":"created"}`)}
	hub := &fakeHub{}
	dispatchRepo, reviewRepo, projectID := newTestRepos(t, clk)

	prov := New(manifest, runner, projectID, "sess-2", dispatchRepo, reviewRepo, hub, clk, nil)
	h, ok := prov.Handler("tool_with_review")
	if !ok {
		t.Fatalf("Handler: tool_with_review not found")
	}

	input := json.RawMessage(`{"value":"hello"}`)
	out, isErr, err := h.Invoke(context.Background(), env0, input)
	if err != nil || isErr {
		t.Fatalf("Invoke: err=%v isErr=%v", err, isErr)
	}
	if out != `{"result":"created"}` {
		t.Errorf("out = %q", out)
	}

	// Review item: status=pending, input/output/draft populated.
	reviews, rErr := reviewRepo.List(projectID, model.ReviewStatusPending, 10, 0)
	if rErr != nil {
		t.Fatalf("List reviews: %v", rErr)
	}
	if len(reviews) != 1 {
		t.Fatalf("review items = %d, want 1", len(reviews))
	}
	rev := reviews[0]
	if rev.Status != model.ReviewStatusPending {
		t.Errorf("review status = %q, want pending", rev.Status)
	}
	if rev.Input != string(input) {
		t.Errorf("review input = %q, want %q", rev.Input, string(input))
	}
	if rev.Output == nil || *rev.Output != out {
		t.Errorf("review output = %v, want %q", rev.Output, out)
	}
	if rev.Draft == nil || *rev.Draft != out {
		t.Errorf("review draft = %v, want %q", rev.Draft, out)
	}

	// Both events broadcast once each.
	events := hub.Events()
	if n := countEvents(events, ws.EventToolDispatched); n != 1 {
		t.Errorf("dispatch events = %d, want 1", n)
	}
	if n := countEvents(events, ws.EventReviewCreated); n != 1 {
		t.Errorf("review events = %d, want 1", n)
	}
	for _, e := range events {
		if e.Type == ws.EventReviewCreated {
			if e.Data["tool_name"] != "tool_with_review" {
				t.Errorf("review event tool_name = %v", e.Data["tool_name"])
			}
			if e.Data["review_item_id"] == "" {
				t.Errorf("review event review_item_id is empty")
			}
		}
	}
}
