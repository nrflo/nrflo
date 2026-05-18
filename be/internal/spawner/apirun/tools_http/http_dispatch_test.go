package tools_http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

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

func countEvents(events []*ws.Event, eventType string) int {
	n := 0
	for _, e := range events {
		if e.Type == eventType {
			n++
		}
	}
	return n
}

func TestHTTPTool_Dispatch_Success(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	respBody := `{"result":"ok"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)

	env := apirun.ToolEnv{
		ProjectID:    projectID,
		WorkflowName: "wf",
		SessionID:    "sess-1",
		Clock:        clk,
		DispatchRepo: dispatchRepo,
		WSHub:        hub,
	}
	h := New(nil)(toolDef("echo-tool", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true output=%q", out)
	}
	if out != respBody {
		t.Errorf("output=%q, want %q", out, respBody)
	}

	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 1 || summary.Success != 1 {
		t.Errorf("dispatch Total=%d Success=%d, want 1/1", summary.Total, summary.Success)
	}

	events := hub.Events()
	if n := countEvents(events, ws.EventToolDispatched); n != 1 {
		t.Errorf("EventToolDispatched count=%d, want 1", n)
	}
	for _, e := range events {
		if e.Type == ws.EventToolDispatched {
			if e.Data["status"] != model.DispatchStatusSuccess {
				t.Errorf("event status=%v, want %q", e.Data["status"], model.DispatchStatusSuccess)
			}
			if e.Data["tool_name"] != "echo-tool" {
				t.Errorf("event tool_name=%v, want echo-tool", e.Data["tool_name"])
			}
		}
	}
}

func TestHTTPTool_Dispatch_500AfterRetry(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`always-bad`))
	}))
	t.Cleanup(srv.Close)

	env := apirun.ToolEnv{
		ProjectID:    projectID,
		WorkflowName: "wf",
		SessionID:    "sess-1",
		Clock:        clk,
		DispatchRepo: dispatchRepo,
		WSHub:        hub,
	}
	h := New(nil)(toolDef("fail-tool", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true on persistent 5xx")
	}
	if !strings.Contains(out, "always-bad") {
		t.Errorf("output=%q, want contains always-bad", out)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("server calls=%d, want 2 (one retry)", n)
	}

	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 1 || summary.Error != 1 {
		t.Errorf("dispatch Total=%d Error=%d, want 1/1", summary.Total, summary.Error)
	}

	events := hub.Events()
	if n := countEvents(events, ws.EventToolDispatched); n != 1 {
		t.Errorf("EventToolDispatched count=%d, want 1", n)
	}
	for _, e := range events {
		if e.Type == ws.EventToolDispatched {
			if e.Data["status"] != model.DispatchStatusError {
				t.Errorf("event status=%v, want %q", e.Data["status"], model.DispatchStatusError)
			}
		}
	}
}
