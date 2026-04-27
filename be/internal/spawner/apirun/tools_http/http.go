// Package tools_http implements the generic HTTP-backed ToolHandler used
// for tool definitions stored in the tool_definitions table. The handler
// POSTs a uniform body and applies the configured auth method.
package tools_http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	maxResponseBytes = 16 * 1024
	truncSuffix      = " ... [truncated]"
	retryDelay       = 500 * time.Millisecond
)

// New returns an apirun.HTTPHandlerFactory bound to a shared http.Client.
// Each handler clones the def pointer so per-tool TimeoutSec/AuthMethod are
// honored at invoke time.
func New(client *http.Client) apirun.HTTPHandlerFactory {
	if client == nil {
		client = &http.Client{}
	}
	return func(def *model.ToolDefinition) apirun.ToolHandler {
		return &httpToolHandler{def: def, client: client}
	}
}

type httpToolHandler struct {
	def    *model.ToolDefinition
	client *http.Client
}

func (h *httpToolHandler) Spec() provider.ToolSpec {
	schema := h.def.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
	}
	return provider.ToolSpec{
		Name:        h.def.Name,
		Description: h.def.Description,
		InputSchema: schema,
	}
}

func (h *httpToolHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	body := map[string]interface{}{
		"tool":  h.def.Name,
		"input": json.RawMessage(input),
		"context": map[string]string{
			"project_id": env.ProjectID,
			"workflow":   env.WorkflowName,
			"session_id": env.SessionID,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Sprintf("marshal request: %s", err.Error()), true, nil
	}

	timeout := time.Duration(h.def.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	authHeader, err := h.resolveAuth(ctx)
	if err != nil {
		return fmt.Sprintf("auth: %s", err.Error()), true, nil
	}

	out, isErr, doErr := h.do(ctx, payload, authHeader, timeout)
	if doErr == nil {
		return out, isErr, nil
	}
	// If do returned a transient error signal (5xx wrapped as retryable), retry once.
	if !isRetryable(doErr) {
		return doErr.Error(), true, nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err().Error(), true, nil
	case <-time.After(retryDelay):
	}
	out, isErr, doErr = h.do(ctx, payload, authHeader, timeout)
	if doErr != nil {
		return doErr.Error(), true, nil
	}
	return out, isErr, nil
}

// retryableErr signals a 5xx that should be retried once.
type retryableErr struct{ err error }

func (r retryableErr) Error() string { return r.err.Error() }

func isRetryable(err error) bool {
	_, ok := err.(retryableErr)
	return ok
}

func (h *httpToolHandler) do(ctx context.Context, payload []byte, authHeader string, timeout time.Duration) (string, bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, h.def.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", true, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", true, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return "", true, fmt.Errorf("read response: %w", readErr)
	}
	out := truncate(string(body))

	switch {
	case resp.StatusCode >= 500:
		return out, true, retryableErr{err: fmt.Errorf("http %d: %s", resp.StatusCode, out)}
	case resp.StatusCode >= 400:
		return out, true, nil
	default:
		return out, false, nil
	}
}

func truncate(s string) string {
	if len(s) <= maxResponseBytes {
		return s
	}
	return s[:maxResponseBytes] + truncSuffix
}

func (h *httpToolHandler) resolveAuth(ctx context.Context) (string, error) {
	method := h.def.AuthMethod
	if method == "" || method == "none" {
		return "", nil
	}
	ref := ""
	if h.def.AuthRef != nil {
		ref = *h.def.AuthRef
	}
	switch method {
	case "bearer_env":
		if ref == "" {
			return "", fmt.Errorf("bearer_env requires auth_ref (env var name)")
		}
		v := os.Getenv(ref)
		if v == "" {
			return "", fmt.Errorf("env var %s is empty", ref)
		}
		return "Bearer " + strings.TrimSpace(v), nil
	case "bearer_secret_ref":
		if ref == "" {
			return "", fmt.Errorf("bearer_secret_ref requires auth_ref")
		}
		v, err := apirun.DereferenceSecretRef(ctx, ref)
		if err != nil {
			return "", err
		}
		return "Bearer " + v, nil
	default:
		return "", fmt.Errorf("unsupported auth_method %q", method)
	}
}
