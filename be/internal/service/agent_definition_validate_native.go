package service

import (
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/types"
)

// normalizeNativeTools trims and re-joins a native_tools CSV, dropping empty
// entries. A non-blank input that normalizes to nothing (e.g. ",,") is
// rejected rather than silently becoming "" (= unrestricted). The
// model.NativeToolsNone sentinel must be the sole entry.
func normalizeNativeTools(csv string) (string, error) {
	if strings.TrimSpace(csv) == "" {
		return "", nil
	}
	var entries []string
	for _, e := range strings.Split(csv, ",") {
		if e = strings.TrimSpace(e); e != "" {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return "", validationErrorf("native_tools: no tool names in %q", csv)
	}
	for _, e := range entries {
		if e == model.NativeToolsNone && len(entries) > 1 {
			return "", validationErrorf("native_tools: %q must be the only entry", model.NativeToolsNone)
		}
	}
	return strings.Join(entries, ","), nil
}

// validateNativeFields hard-rejects native_tools/sandbox values that cannot
// apply to the def: native_tools is claude-only (cli_interactive + anthropic
// provider), sandbox is codex-only (cli_interactive + openai provider). A
// model row that no longer resolves is left to the caller's own model
// validation (same leniency as validateDefReasoningEffort).
func validateNativeFields(modelSvc *ModelService, executionMode, modelName, nativeTools, sandbox string) error {
	if !model.ValidSandbox(sandbox) {
		return validationErrorf("invalid sandbox: must be %q, %q or %q",
			model.SandboxReadOnly, model.SandboxWorkspaceWrite, model.SandboxDangerFullAccess)
	}
	if nativeTools == "" && sandbox == "" {
		return nil
	}
	if executionMode != "cli_interactive" {
		if nativeTools != "" {
			return validationErrorf("native_tools requires execution_mode=cli_interactive")
		}
		return validationErrorf("sandbox requires execution_mode=cli_interactive")
	}
	m, err := modelSvc.Get(modelName)
	if err != nil {
		return nil
	}
	if nativeTools != "" && m.Provider != "anthropic" {
		return validationErrorf("native_tools is only supported for anthropic (claude) models, model %q has provider %q", modelName, m.Provider)
	}
	if sandbox != "" && m.Provider != "openai" {
		return validationErrorf("sandbox is only supported for openai (codex) models, model %q has provider %q", modelName, m.Provider)
	}
	return nil
}

// revalidateNativeFields re-checks native_tools/sandbox against the effective
// values (current row merged with the incoming update) whenever model,
// execution_mode, native_tools, or sandbox changes on a PATCH — so swapping
// the model to another provider with a stale non-empty restriction is
// rejected unless the same PATCH clears it.
func (s *AgentDefinitionService) revalidateNativeFields(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) error {
	if req.Model == nil && req.ExecutionMode == nil && req.NativeTools == nil && req.Sandbox == nil {
		return nil
	}
	var currentModel, currentMode, currentNativeTools, currentSandbox string
	if queryErr := s.pool.QueryRow(
		"SELECT model, execution_mode, native_tools, sandbox FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&currentModel, &currentMode, &currentNativeTools, &currentSandbox); queryErr != nil {
		return fmt.Errorf("failed to load agent definition: %w", queryErr)
	}
	effectiveModel := currentModel
	if req.Model != nil {
		effectiveModel = *req.Model
	}
	effectiveMode := currentMode
	if req.ExecutionMode != nil {
		effectiveMode = *req.ExecutionMode
	}
	effectiveNativeTools := currentNativeTools
	if req.NativeTools != nil {
		effectiveNativeTools = *req.NativeTools
	}
	effectiveSandbox := currentSandbox
	if req.Sandbox != nil {
		effectiveSandbox = *req.Sandbox
	}
	return validateNativeFields(s.modelSvc, effectiveMode, effectiveModel, effectiveNativeTools, effectiveSandbox)
}
