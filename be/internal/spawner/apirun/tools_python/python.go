// Package tools_python implements apirun.ToolHandler for python_scripts rows
// with kind=tool. The script is executed via the per-project python interpreter
// with input fed on stdin and output read from stdout.
package tools_python

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

const (
	maxOutputBytes    = 16 * 1024
	outputTruncSuffix = " ... (truncated)"
	defaultTimeoutSec = 30
)

// PythonToolHandler implements apirun.ToolHandler for python_scripts rows with kind=tool.
type PythonToolHandler struct {
	row        *model.PythonScript
	pythonPath string
	projectEnv []string
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
}

// New creates a PythonToolHandler. pythonPath defaults to "python3" when empty,
// mirroring resolvePythonBin in spawner/backend_script.go.
func New(row *model.PythonScript, pythonPath string, projectEnv []string) *PythonToolHandler {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	return &PythonToolHandler{
		row:        row,
		pythonPath: pythonPath,
		projectEnv: projectEnv,
	}
}

// Spec returns the ToolSpec for this handler. Empty InputSchema falls back to
// the permissive default, mirroring tools_http/http.go:46-49.
func (h *PythonToolHandler) Spec() provider.ToolSpec {
	schema := json.RawMessage(h.row.InputSchema)
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
	}
	return provider.ToolSpec{
		Name:        h.row.Name,
		Description: h.row.ToolDescription,
		InputSchema: schema,
	}
}

func (h *PythonToolHandler) compileSchema() (*jsonschema.Schema, error) {
	h.schemaOnce.Do(func() {
		s := h.row.InputSchema
		if s == "" {
			s = `{"type":"object","properties":{},"additionalProperties":true}`
		}
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		if err := compiler.AddResource("schema://tool", strings.NewReader(s)); err != nil {
			h.schemaErr = fmt.Errorf("compile schema: %w", err)
			return
		}
		compiled, err := compiler.Compile("schema://tool")
		if err != nil {
			h.schemaErr = fmt.Errorf("compile schema: %w", err)
			return
		}
		h.schema = compiled
	})
	return h.schema, h.schemaErr
}

// Invoke executes the python script with input on stdin and captures stdout.
// It never returns a Go error — failures are surfaced as (output, isError=true, nil).
func (h *PythonToolHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	compiled, compileErr := h.compileSchema()
	if compileErr != nil {
		out := fmt.Sprintf("schema compile error: %s", compileErr.Error())
		h.recordDispatch(env, input, out, true, 0)
		return out, true, nil
	}

	if compiled != nil {
		var inputVal interface{}
		if len(input) > 0 {
			if err := json.Unmarshal(input, &inputVal); err != nil {
				out := fmt.Sprintf("input is not valid JSON: %s", err.Error())
				h.recordDispatch(env, input, out, true, 0)
				return out, true, nil
			}
		}
		if err := compiled.Validate(inputVal); err != nil {
			out := fmt.Sprintf("schema validation failed: %s", err.Error())
			h.recordDispatch(env, input, out, true, 0)
			return out, true, nil
		}
	}

	scriptBytes, readErr := h.resolveScript()
	if readErr != nil {
		out := fmt.Sprintf("resolve script: %s", readErr.Error())
		h.recordDispatch(env, input, out, true, 0)
		return out, true, nil
	}

	tmp, err := os.CreateTemp("", "nrflo-tool-*.py")
	if err != nil {
		out := fmt.Sprintf("create temp file: %s", err.Error())
		h.recordDispatch(env, input, out, true, 0)
		return out, true, nil
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(scriptBytes); err != nil {
		tmp.Close()
		out := fmt.Sprintf("write temp file: %s", err.Error())
		h.recordDispatch(env, input, out, true, 0)
		return out, true, nil
	}
	tmp.Close()

	timeoutSec := h.row.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, h.pythonPath, tmpName)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = h.buildEnv(ctx, env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := env.Clock.Now()
	runErr := cmd.Run()
	durationMs := env.Clock.Now().Sub(start).Milliseconds()

	var out string
	isError := false

	switch {
	case timeoutCtx.Err() == context.DeadlineExceeded:
		out = fmt.Sprintf("tool timed out after %ds", timeoutSec)
		isError = true
	case runErr != nil:
		out = stderr.String()
		isError = true
	default:
		out = stdout.String()
		if len(out) > maxOutputBytes {
			out = out[:maxOutputBytes] + outputTruncSuffix
		}
	}

	h.recordDispatch(env, input, out, isError, durationMs)
	return out, isError, nil
}

// resolveScript returns script bytes: FilePath takes precedence over Code,
// mirroring spawner.prepareScriptSpawn validation order.
func (h *PythonToolHandler) resolveScript() ([]byte, error) {
	fp := h.row.FilePath
	if fp != "" && filepath.IsAbs(fp) && strings.HasSuffix(fp, ".py") {
		return os.ReadFile(fp)
	}
	return []byte(h.row.Code), nil
}

// buildEnv assembles the child process environment. nrflo-controlled vars come
// first; projectEnv trails so project-supplied values win on key collision.
func (h *PythonToolHandler) buildEnv(ctx context.Context, env apirun.ToolEnv) []string {
	e := []string{
		"NRFLO_PROJECT=" + env.ProjectID,
		"NRF_SESSION_ID=" + env.SessionID,
		"NRF_WORKFLOW_INSTANCE_ID=" + env.WorkflowInstanceID,
		"NRF_TRX=" + logger.TrxFromContext(ctx),
		"NRF_SPAWNED=1",
	}
	return append(e, h.projectEnv...)
}

func (h *PythonToolHandler) recordDispatch(env apirun.ToolEnv, input json.RawMessage, output string, isError bool, durationMs int64) {
	status := model.DispatchStatusSuccess
	var errMsg *string
	var outPtr *string
	if isError {
		status = model.DispatchStatusError
		s := output
		errMsg = &s
	} else {
		s := output
		outPtr = &s
	}
	sessionID := env.SessionID
	dispatch := &model.ToolDispatch{
		ProjectID:  env.ProjectID,
		SessionID:  &sessionID,
		ToolName:   h.row.Name,
		Input:      string(input),
		Output:     outPtr,
		Status:     status,
		ErrorMsg:   errMsg,
		DurationMs: durationMs,
	}
	var dispatchID string
	if env.DispatchRepo != nil {
		if insertErr := env.DispatchRepo.Insert(dispatch); insertErr == nil {
			dispatchID = dispatch.ID
		}
	}
	if env.WSHub != nil {
		env.WSHub.Broadcast(ws.NewEvent(ws.EventToolDispatched, env.ProjectID, env.TicketID, env.WorkflowName, map[string]interface{}{
			"tool_name":   h.row.Name,
			"status":      status,
			"duration_ms": durationMs,
			"dispatch_id": dispatchID,
		}))
	}
}
