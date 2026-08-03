package apirun

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"be/internal/model"
	"be/internal/spawner/apirun/provider"
)

// selfRecordingHandler is implemented by tool handlers that already write
// their own tool_dispatches row (tools_python.PythonToolHandler,
// source=model.DispatchSourcePython) — WrapToolAudit skips those to avoid a
// duplicate row, pushing the divergence into the handler rather than a
// name/type check at the wrap site (root CLAUDE.md rule 6).
type selfRecordingHandler interface {
	RecordsDispatchItself()
}

// WrapToolAudit wraps every handler in reg with a toolAuditDecorator,
// skipping handlers that self-record, and preserving MediaToolHandler for
// handlers that implement it (dropping it would silently strip media
// content blocks, e.g. read_document). Applied once at registry-build time
// (spawner_api_registry.go, console/registry.go) so every invoke site
// (CLI-MCP DispatchTool, API-mode Runner.invokeTool, console Dispatch)
// writes exactly one tool_dispatches row per call with zero call-site edits
// — the decorator itself is stateless and reads Source/SessionKind/
// DispatchRepo from the ToolEnv passed to Invoke, not from anything
// captured at wrap time.
func WrapToolAudit(reg Registry) Registry {
	out := make(Registry, len(reg))
	for name, h := range reg {
		if _, self := h.(selfRecordingHandler); self {
			out[name] = h
			continue
		}
		base := toolAuditDecorator{inner: h}
		if _, ok := h.(MediaToolHandler); ok {
			out[name] = toolAuditMediaDecorator{toolAuditDecorator: base}
		} else {
			out[name] = base
		}
	}
	return out
}

// toolAuditDecorator times one Invoke call and writes one tool_dispatches
// row via env.DispatchRepo (no-op when nil). TerminalSignal passes through
// untouched via errors.As — swallowing it would break agent_fail/
// agent_continue/agent_callback.
type toolAuditDecorator struct {
	inner ToolHandler
}

func (d toolAuditDecorator) Spec() provider.ToolSpec { return d.inner.Spec() }

func (d toolAuditDecorator) Invoke(ctx context.Context, env ToolEnv, input json.RawMessage) (string, bool, error) {
	start := time.Now()
	output, isError, err := d.inner.Invoke(ctx, env, input)
	d.record(env, input, output, isError, err, start)
	return output, isError, err
}

func (d toolAuditDecorator) record(env ToolEnv, input json.RawMessage, output string, isError bool, err error, start time.Time) {
	if env.DispatchRepo == nil {
		return
	}
	var ts TerminalSignal
	terminal := errors.As(err, &ts)

	status := model.DispatchStatusSuccess
	var errMsg *string
	var outPtr *string
	switch {
	case terminal:
		outPtr = strPtr(ts.Reason)
	case err != nil:
		status = model.DispatchStatusError
		errMsg = strPtr(err.Error())
	case isError:
		status = model.DispatchStatusError
		errMsg = strPtr(output)
	default:
		outPtr = strPtr(output)
	}

	sessionID := env.SessionID
	dispatch := &model.ToolDispatch{
		ProjectID:          env.ProjectID,
		SessionID:          &sessionID,
		ToolName:           d.inner.Spec().Name,
		Input:              string(input),
		Output:             outPtr,
		Status:             status,
		ErrorMsg:           errMsg,
		DurationMs:         time.Since(start).Milliseconds(),
		Source:             env.Source,
		SessionKind:        env.SessionKind,
		WorkflowInstanceID: env.WorkflowInstanceID,
	}
	_ = env.DispatchRepo.Insert(dispatch)
}

// toolAuditMediaDecorator additionally implements MediaToolHandler so the
// runner's `handler.(MediaToolHandler)` type assertion keeps seeing media
// support through the decorator.
type toolAuditMediaDecorator struct {
	toolAuditDecorator
}

func (d toolAuditMediaDecorator) InvokeMedia(ctx context.Context, env ToolEnv, input json.RawMessage) (string, []provider.MediaBlock, bool, error) {
	mh := d.inner.(MediaToolHandler) //nolint:forcetypeassert // guarded by WrapToolAudit's construction
	start := time.Now()
	output, media, isError, err := mh.InvokeMedia(ctx, env, input)
	d.record(env, input, output, isError, err, start)
	return output, media, isError, err
}

func strPtr(s string) *string { return &s }
