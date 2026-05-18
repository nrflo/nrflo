package tools_python

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

func pythonRow(name, code, schema string, timeoutSec int) *model.PythonScript {
	return &model.PythonScript{
		Name:            name,
		Code:            code,
		InputSchema:     schema,
		TimeoutSec:      timeoutSec,
		ToolDescription: "test tool",
	}
}

func testEnv(t *testing.T, clk clock.Clock, dispatchRepo *repo.DispatchRepo, projectID string, hub *fakeHub) apirun.ToolEnv {
	t.Helper()
	return apirun.ToolEnv{
		ProjectID:    projectID,
		WorkflowName: "wf",
		SessionID:    "sess-1",
		Clock:        clk,
		DispatchRepo: dispatchRepo,
		WSHub:        hub,
	}
}

func TestPython_SchemaValidationFails(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	schema := `{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`
	row := pythonRow("schema-tool", `import json,sys; print(json.dumps({"ok":True}))`, schema, 5)
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true on schema validation failure")
	}
	if !strings.Contains(out, "schema validation failed") {
		t.Errorf("output=%q, want contains 'schema validation failed'", out)
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
				t.Errorf("event status=%v, want error", e.Data["status"])
			}
		}
	}
}

func TestPython_Success_StdinStdoutRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	code := `import json,sys; d=json.loads(sys.stdin.read()); print(json.dumps({"echo": d["q"]}))`
	row := pythonRow("echo-tool", code, "", 10)
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{"q":"hi"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true output=%q", out)
	}
	if !strings.Contains(out, `"echo"`) || !strings.Contains(out, `"hi"`) {
		t.Errorf("output=%q, want contains echo:hi", out)
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
				t.Errorf("event status=%v, want success", e.Data["status"])
			}
		}
	}
}

func TestPython_NonZeroExit_StderrSurfaces(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	code := `import sys; print("boom", file=sys.stderr); sys.exit(2)`
	row := pythonRow("fail-tool", code, "", 10)
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true on non-zero exit")
	}
	if out != "boom\n" {
		t.Errorf("output=%q, want %q", out, "boom\n")
	}

	summary, sErr := dispatchRepo.ListSummary(projectID, since)
	if sErr != nil {
		t.Fatalf("ListSummary: %v", sErr)
	}
	if summary.Total != 1 || summary.Error != 1 {
		t.Errorf("dispatch Total=%d Error=%d, want 1/1", summary.Total, summary.Error)
	}
}

func TestPython_Timeout(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}
	since := clk.Now().Add(-time.Second)

	code := `import time; time.sleep(5)`
	row := pythonRow("sleep-tool", code, "", 1)
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true on timeout")
	}
	if !strings.Contains(out, "tool timed out after 1s") {
		t.Errorf("output=%q, want contains 'tool timed out after 1s'", out)
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
				t.Errorf("event status=%v, want error", e.Data["status"])
			}
		}
	}
}

func TestPython_Truncates16KB(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}

	code := `import sys; sys.stdout.write("a"*20480); sys.stdout.flush()`
	row := pythonRow("big-tool", code, "", 10)
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true output=%q", out[:min(80, len(out))])
	}
	wantLen := maxOutputBytes + len(outputTruncSuffix)
	if len(out) != wantLen {
		t.Errorf("len(out)=%d, want %d", len(out), wantLen)
	}
	if !strings.HasSuffix(out, outputTruncSuffix) {
		t.Errorf("output missing truncation suffix; got suffix %q", out[len(out)-20:])
	}
}

func TestPython_FilePathPreferredOverCode(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)
	hub := &fakeHub{}

	dir := t.TempDir()
	pyFile := filepath.Join(dir, "tool.py")
	if err := os.WriteFile(pyFile, []byte(`print("from_file")`), 0644); err != nil {
		t.Fatalf("write py file: %v", err)
	}

	row := &model.PythonScript{
		Name:            "file-tool",
		Code:            `print("from_code")`,
		FilePath:        pyFile,
		ToolDescription: "test",
		TimeoutSec:      10,
	}
	h := New(row, "", nil)
	env := testEnv(t, clk, dispatchRepo, projectID, hub)

	out, isErr, err := h.Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true output=%q", out)
	}
	if !strings.Contains(out, "from_file") {
		t.Errorf("output=%q, want contains 'from_file' (FilePath must take precedence over Code)", out)
	}
	if strings.Contains(out, "from_code") {
		t.Errorf("output=%q, want Code not used when FilePath set", out)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
