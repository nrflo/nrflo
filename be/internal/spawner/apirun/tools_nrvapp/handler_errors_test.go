package tools_nrvapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/nrvapp/python"
	"be/internal/ws"
)

func TestManifestHandler_ScriptError(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	scriptErr := &python.ScriptError{ExitCode: 1, Stderr: "command failed"}
	runner := &fakeRunner{err: scriptErr}
	hub := &fakeHub{}
	dispatchRepo, reviewRepo, projectID := newTestRepos(t, clk)
	since := clk.Now().Add(-time.Second)

	// tool_with_review has review:true, but error path must skip review creation.
	prov := New(manifest, runner, projectID, "sess-3", dispatchRepo, reviewRepo, hub, clk)
	h, ok := prov.Handler("tool_with_review")
	if !ok {
		t.Fatalf("Handler: tool_with_review not found")
	}

	out, isErr, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil {
		t.Fatalf("Invoke returned non-nil error: %v", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true on script error")
	}
	if out == "" {
		t.Errorf("out is empty, want error message")
	}

	// Dispatch row with status=error.
	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 1 || summary.Error != 1 {
		t.Errorf("dispatch Total=%d Error=%d, want 1/1", summary.Total, summary.Error)
	}

	// No review row inserted on error.
	reviews, rErr := reviewRepo.List(projectID, "", 10, 0)
	if rErr != nil {
		t.Fatalf("List reviews: %v", rErr)
	}
	if len(reviews) != 0 {
		t.Errorf("review items = %d, want 0 (error skips review)", len(reviews))
	}

	// EventNrvappDispatchCompleted with status=error; no review event.
	events := hub.Events()
	if n := countEvents(events, ws.EventNrvappDispatchCompleted); n != 1 {
		t.Errorf("dispatch events = %d, want 1", n)
	}
	if n := countEvents(events, ws.EventNrvappReviewCreated); n != 0 {
		t.Errorf("review events = %d, want 0 on error", n)
	}
	for _, e := range events {
		if e.Type == ws.EventNrvappDispatchCompleted {
			if e.Data["status"] != model.DispatchStatusError {
				t.Errorf("dispatch event status = %v, want error", e.Data["status"])
			}
		}
	}
}

func TestManifestHandler_ValidationError(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`{"result":"ok"}`)}
	hub := &fakeHub{}
	dispatchRepo, reviewRepo, projectID := newTestRepos(t, clk)
	since := clk.Now().Add(-time.Second)

	prov := New(manifest, runner, projectID, "sess-4", dispatchRepo, reviewRepo, hub, clk)
	h, ok := prov.Handler("tool_no_review")
	if !ok {
		t.Fatalf("Handler: tool_no_review not found")
	}

	// Missing required field "value" → input schema validation error.
	out, isErr, err := h.Invoke(context.Background(), env0, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke returned non-nil error: %v", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true on validation error")
	}
	if out == "" {
		t.Errorf("out is empty, want validation error message")
	}

	// Runner was NOT called.
	if runner.Calls() != 0 {
		t.Errorf("runner calls = %d, want 0 (validation failure short-circuits)", runner.Calls())
	}

	// No dispatch row.
	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 0 {
		t.Errorf("dispatch Total = %d, want 0 (validation error skips insert)", summary.Total)
	}

	// No broadcasts.
	if n := len(hub.Events()); n != 0 {
		t.Errorf("hub events = %d, want 0 on validation error", n)
	}
}

func TestManifestHandler_InvalidJSON_Input(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`"ok"`)}
	hub := &fakeHub{}
	dispatchRepo, reviewRepo, projectID := newTestRepos(t, clk)

	prov := New(manifest, runner, projectID, "sess-5", dispatchRepo, reviewRepo, hub, clk)
	h, ok := prov.Handler("tool_no_review")
	if !ok {
		t.Fatalf("Handler: tool_no_review not found")
	}

	// Malformed JSON input.
	out, isErr, err := h.Invoke(context.Background(), env0, json.RawMessage(`not-json`))
	if err != nil {
		t.Fatalf("Invoke returned non-nil error: %v", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true on invalid JSON")
	}
	if !strings.Contains(out, "not valid JSON") {
		t.Errorf("out = %q, want 'not valid JSON'", out)
	}
	if runner.Calls() != 0 {
		t.Errorf("runner calls = %d, want 0 on invalid JSON", runner.Calls())
	}
}

func TestManifestProvider_Specs(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{}
	hub := &fakeHub{}

	prov := New(manifest, runner, "proj-1", "sess-6", nil, nil, hub, clk)
	specs := prov.Specs()

	// Manifest has two tools.
	if len(specs) != 2 {
		t.Fatalf("Specs() = %d entries, want 2", len(specs))
	}
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
		if len(s.InputSchema) == 0 {
			t.Errorf("spec %q has empty InputSchema", s.Name)
		}
	}
	if !names["tool_no_review"] {
		t.Errorf("tool_no_review missing from specs")
	}
	if !names["tool_with_review"] {
		t.Errorf("tool_with_review missing from specs")
	}
}

func TestManifestProvider_Handler_NotFound(t *testing.T) {
	manifest := newTestManifest(t)
	prov := New(manifest, &fakeRunner{}, "proj-1", "sess-7", nil, nil, nil, clock.NewTest(time.Now()))
	_, ok := prov.Handler("nonexistent_tool")
	if ok {
		t.Errorf("Handler returned true for unknown tool, want false")
	}
}

func TestManifestHandler_NilRepos_NoRepoInsert(t *testing.T) {
	// When repos are nil, no panic; just skips DB inserts.
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newTestManifest(t)
	runner := &fakeRunner{out: []byte(`"ok"`)}
	hub := &fakeHub{}

	prov := New(manifest, runner, "proj-1", "sess-8", nil, nil, hub, clk)
	h, ok := prov.Handler("tool_with_review")
	if !ok {
		t.Fatalf("Handler: tool_with_review not found")
	}

	_, isErr, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil || isErr {
		t.Fatalf("Invoke with nil repos: err=%v isErr=%v", err, isErr)
	}
	// Dispatch event still broadcast (hub is non-nil).
	if n := countEvents(hub.Events(), ws.EventNrvappDispatchCompleted); n != 1 {
		t.Errorf("dispatch events = %d, want 1", n)
	}
}
