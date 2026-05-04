package tools_manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/manifest/config"
	"be/internal/manifest/python"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

const defaultScriptTimeout = 30 * time.Second

// manifestProvider implements apirun.ManifestProvider for a single manifest.
type manifestProvider struct {
	deps deps
}

// New creates a ManifestProvider backed by the given manifest. All dependencies
// are required — callers that cannot supply a repo should pass nil for that
// repo (Insert calls are skipped gracefully).
func New(
	manifest *config.Manifest,
	runner python.Runner,
	projectID string,
	sessionID string,
	dispatchRepo *repo.DispatchRepo,
	reviewRepo *repo.ReviewRepo,
	hub service.WSHub,
	clk clock.Clock,
) apirun.ManifestProvider {
	return &manifestProvider{deps: deps{
		manifest:     manifest,
		runner:       runner,
		projectID:    projectID,
		sessionID:    sessionID,
		dispatchRepo: dispatchRepo,
		reviewRepo:   reviewRepo,
		hub:          hub,
		clock:        clk,
	}}
}

// Specs returns provider.ToolSpec for every tool in the manifest.
func (p *manifestProvider) Specs() []provider.ToolSpec {
	specs := make([]provider.ToolSpec, 0, len(p.deps.manifest.Tools))
	for i := range p.deps.manifest.Tools {
		t := &p.deps.manifest.Tools[i]
		specs = append(specs, provider.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: json.RawMessage(t.InputSchema),
		})
	}
	return specs
}

// Handler returns the ToolHandler for the named tool, if it exists.
func (p *manifestProvider) Handler(name string) (apirun.ToolHandler, bool) {
	tool, ok := p.deps.manifest.Tool(name)
	if !ok {
		return nil, false
	}
	return &manifestToolHandler{tool: tool, d: p.deps}, true
}

// manifestToolHandler implements apirun.ToolHandler for a single manifest tool.
type manifestToolHandler struct {
	tool *config.Tool
	d    deps
}

func (h *manifestToolHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        h.tool.Name,
		Description: h.tool.Description,
		InputSchema: json.RawMessage(h.tool.InputSchema),
	}
}

func (h *manifestToolHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	// Validate input against the tool's JSON schema. Unmarshal first because
	// ValidateInput expects an interface{} value, not raw JSON bytes.
	var inputVal interface{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &inputVal); err != nil {
			return fmt.Sprintf("input is not valid JSON: %s", err.Error()), true, nil
		}
	}
	if err := h.d.manifest.ValidateInput(h.tool.Name, inputVal); err != nil {
		return fmt.Sprintf("input validation error: %s", err.Error()), true, nil
	}

	start := h.d.clock.Now()

	// Invoke the Python script.
	rt := python.NewRuntime(h.d.runner, h.d.manifest.Dir)
	envVars := python.MatchEnv(h.tool.EnvAllow, os.Environ())
	out, runErr := rt.Invoke(ctx, h.tool.Script, []byte(input), envVars, defaultScriptTimeout)

	durationMs := h.d.clock.Now().Sub(start).Milliseconds()

	status := model.DispatchStatusSuccess
	var errMsg *string
	var outStr *string
	isError := false

	if runErr != nil {
		status = model.DispatchStatusError
		msg := runErr.Error()
		errMsg = &msg
		isError = true
	} else {
		s := string(out)
		outStr = &s
	}

	// Persist dispatch row.
	sessionIDPtr := &h.d.sessionID
	dispatch := &model.ToolDispatch{
		ProjectID:  h.d.projectID,
		SessionID:  sessionIDPtr,
		ToolName:   h.tool.Name,
		Input:      string(input),
		Output:     outStr,
		Status:     status,
		ErrorMsg:   errMsg,
		DurationMs: durationMs,
	}
	var dispatchID string
	if h.d.dispatchRepo != nil {
		if insertErr := h.d.dispatchRepo.Insert(dispatch); insertErr == nil {
			dispatchID = dispatch.ID
		}
	}

	// Broadcast dispatch completed event.
	if h.d.hub != nil {
		h.d.hub.Broadcast(ws.NewEvent(ws.EventToolDispatched, h.d.projectID, "", "", map[string]interface{}{
			"tool_name":   h.tool.Name,
			"status":      status,
			"duration_ms": durationMs,
			"dispatch_id": dispatchID,
		}))
	}

	// On error: return early without creating a review item.
	if isError {
		return *errMsg, true, nil
	}

	// Optionally create a review item.
	if h.tool.Review && h.d.reviewRepo != nil {
		outVal := string(out)
		item := &model.ReviewItem{
			ProjectID: h.d.projectID,
			ToolName:  h.tool.Name,
			SessionID: sessionIDPtr,
			Input:     string(input),
			Output:    &outVal,
			Draft:     &outVal,
			Status:    model.ReviewStatusPending,
		}
		if insertErr := h.d.reviewRepo.Insert(item); insertErr == nil && h.d.hub != nil {
			h.d.hub.Broadcast(ws.NewEvent(ws.EventReviewCreated, h.d.projectID, "", "", map[string]interface{}{
				"review_item_id": item.ID,
				"tool_name":      h.tool.Name,
			}))
		}
	}

	return string(out), false, nil
}
