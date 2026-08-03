package apirun

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider"
)

// auditHandler is a minimal ToolHandler stub for WrapToolAudit tests.
type auditHandler struct {
	name     string
	output   string
	isError  bool
	err      error
	terminal *TerminalSignal
}

func (h *auditHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *auditHandler) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	if h.terminal != nil {
		return "", false, *h.terminal
	}
	return h.output, h.isError, h.err
}

// mediaAuditHandler additionally implements MediaToolHandler.
type mediaAuditHandler struct {
	auditHandler
	media []provider.MediaBlock
}

func (h *mediaAuditHandler) InvokeMedia(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, []provider.MediaBlock, bool, error) {
	return h.output, h.media, h.isError, h.err
}

// selfRecordingAuditHandler implements the selfRecordingHandler marker.
type selfRecordingAuditHandler struct {
	auditHandler
}

func (h *selfRecordingAuditHandler) RecordsDispatchItself() {}

func newAuditTestEnv(t *testing.T, source, sessionKind string) (Registry, ToolEnv, *repo.DispatchRepo) {
	t.Helper()
	pool := openAPIRunTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-audit', 'P', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	dispatchRepo := repo.NewDispatchRepo(pool, clock.Real())
	env := ToolEnv{
		ProjectID:    "proj-audit",
		SessionID:    "sess-audit",
		DispatchRepo: dispatchRepo,
		Source:       source,
		SessionKind:  sessionKind,
	}
	return Registry{}, env, dispatchRepo
}

func countDispatchRows(t *testing.T, dr *repo.DispatchRepo, sessionID string) []*model.ToolDispatch {
	t.Helper()
	rows, err := dr.ListBySession(sessionID, 0)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	return rows
}

func TestWrapToolAudit_SuccessfulInvoke_WritesOneRow(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceHTTP, model.AgentSessionKindWorkflowAgent)
	reg := WrapToolAudit(Registry{"ok_tool": &auditHandler{name: "ok_tool", output: `{"result":"fine"}`}})

	out, isErr, err := reg["ok_tool"].Invoke(context.Background(), env, json.RawMessage(`{"a":1}`))
	if err != nil || isErr {
		t.Fatalf("Invoke() = (%q, %v, %v), want success", out, isErr, err)
	}

	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Status != model.DispatchStatusSuccess {
		t.Errorf("Status = %q, want success", row.Status)
	}
	if row.Source != model.DispatchSourceHTTP || row.SessionKind != model.AgentSessionKindWorkflowAgent {
		t.Errorf("Source/SessionKind = %q/%q, want %q/%q", row.Source, row.SessionKind, model.DispatchSourceHTTP, model.AgentSessionKindWorkflowAgent)
	}
	if row.Output == nil || *row.Output != `{"result":"fine"}` {
		t.Errorf("Output = %v, want the handler's output", row.Output)
	}
}

func TestWrapToolAudit_IsErrorResult_RecordsErrorStatus(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceMCP, model.AgentSessionKindWorkflowAgent)
	reg := WrapToolAudit(Registry{"bad_tool": &auditHandler{name: "bad_tool", output: "boom", isError: true}})

	if _, _, err := reg["bad_tool"].Invoke(context.Background(), env, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke() error = %v, want nil (isError, not a Go error)", err)
	}

	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Status != model.DispatchStatusError {
		t.Errorf("Status = %q, want error", rows[0].Status)
	}
	if rows[0].ErrorMsg == nil || *rows[0].ErrorMsg != "boom" {
		t.Errorf("ErrorMsg = %v, want boom", rows[0].ErrorMsg)
	}
}

func TestWrapToolAudit_GoError_RecordsErrorStatus(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceMCP, model.AgentSessionKindWorkflowAgent)
	reg := WrapToolAudit(Registry{"err_tool": &auditHandler{name: "err_tool", err: errors.New("dial failed")}})

	if _, _, err := reg["err_tool"].Invoke(context.Background(), env, json.RawMessage(`{}`)); err == nil {
		t.Fatal("Invoke() error = nil, want dial failed")
	}

	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Status != model.DispatchStatusError || rows[0].ErrorMsg == nil || *rows[0].ErrorMsg != "dial failed" {
		t.Errorf("row = %+v, want error status with dial failed", rows[0])
	}
}

func TestWrapToolAudit_TerminalSignal_PassesThroughUnrecorded_AsReason(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceMCP, model.AgentSessionKindWorkflowAgent)
	ts := TerminalSignal{Status: "FAIL", Reason: "explicit fail"}
	reg := WrapToolAudit(Registry{"agent_fail": &auditHandler{name: "agent_fail", terminal: &ts}})

	_, _, err := reg["agent_fail"].Invoke(context.Background(), env, json.RawMessage(`{}`))
	var got TerminalSignal
	if !errors.As(err, &got) {
		t.Fatalf("Invoke() error = %v, want a TerminalSignal to pass through untouched", err)
	}
	if got.Status != "FAIL" || got.Reason != "explicit fail" {
		t.Errorf("TerminalSignal = %+v, want FAIL/explicit fail (untouched)", got)
	}

	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (a dispatch row is still recorded)", len(rows))
	}
	if rows[0].Output == nil || *rows[0].Output != "explicit fail" {
		t.Errorf("Output = %v, want the terminal signal's reason recorded, not swallowed", rows[0].Output)
	}
}

func TestWrapToolAudit_MediaHandler_PreservedThroughDecorator(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceHTTP, model.AgentSessionKindWorkflowAgent)
	media := []provider.MediaBlock{{Kind: "image", MediaType: "image/png", DataB64: "ZmFrZQ=="}}
	reg := WrapToolAudit(Registry{"read_document": &mediaAuditHandler{
		auditHandler: auditHandler{name: "read_document", output: "ok"},
		media:        media,
	}})

	h, ok := reg["read_document"].(MediaToolHandler)
	if !ok {
		t.Fatal("wrapped handler does not implement MediaToolHandler, want it preserved")
	}
	out, gotMedia, isErr, err := h.InvokeMedia(context.Background(), env, json.RawMessage(`{}`))
	if err != nil || isErr || out != "ok" {
		t.Fatalf("InvokeMedia() = (%q, %v, %v), want ok/false/nil", out, isErr, err)
	}
	if len(gotMedia) != 1 || gotMedia[0].MediaType != "image/png" {
		t.Errorf("media = %+v, want the original image block preserved", gotMedia)
	}

	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 1 {
		t.Errorf("len(rows) = %d, want 1 (InvokeMedia also records a dispatch row)", len(rows))
	}
}

func TestWrapToolAudit_SelfRecordingHandler_NotWrapped_NoDuplicateRow(t *testing.T) {
	t.Parallel()
	_, env, dr := newAuditTestEnv(t, model.DispatchSourceHTTP, model.AgentSessionKindWorkflowAgent)
	inner := &selfRecordingAuditHandler{auditHandler: auditHandler{name: "python_tool", output: "ok"}}
	reg := WrapToolAudit(Registry{"python_tool": inner})

	if reg["python_tool"] != ToolHandler(inner) {
		t.Error("self-recording handler was wrapped, want it passed through unchanged")
	}

	if _, _, err := reg["python_tool"].Invoke(context.Background(), env, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rows := countDispatchRows(t, dr, "sess-audit")
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0 (self-recording handlers are skipped by WrapToolAudit)", len(rows))
	}
}

func TestWrapToolAudit_NilDispatchRepo_NoOp(t *testing.T) {
	t.Parallel()
	env := ToolEnv{ProjectID: "proj-audit", SessionID: "sess-audit"} // DispatchRepo nil
	reg := WrapToolAudit(Registry{"ok_tool": &auditHandler{name: "ok_tool", output: "fine"}})

	out, isErr, err := reg["ok_tool"].Invoke(context.Background(), env, json.RawMessage(`{}`))
	if err != nil || isErr || out != "fine" {
		t.Fatalf("Invoke() = (%q, %v, %v), want fine/false/nil even with nil DispatchRepo", out, isErr, err)
	}
}
