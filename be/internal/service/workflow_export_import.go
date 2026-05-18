package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/types"
)

// CheckImport probes the target project for conflicts with the bundle.
func (s *WorkflowExportService) CheckImport(projectID string, bundle *types.WorkflowBundle) (*types.ImportConflicts, error) {
	conflicts := &types.ImportConflicts{
		WorkflowIDs:     []string{},
		PythonScriptIDs: []string{},
	}

	for _, entry := range bundle.Workflows {
		if entry.Workflow == nil {
			continue
		}
		_, err := s.workflowSvc.GetWorkflowDef(projectID, entry.Workflow.ID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			return nil, err
		}
		conflicts.WorkflowIDs = append(conflicts.WorkflowIDs, entry.Workflow.ID)
	}

	for _, script := range bundle.PythonScripts {
		if script == nil {
			continue
		}
		_, err := s.pythonScriptSvc.Get(projectID, script.ID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			return nil, err
		}
		conflicts.PythonScriptIDs = append(conflicts.PythonScriptIDs, script.ID)
	}

	return conflicts, nil
}

// Import applies the bundle to the target project according to action.
// Actions: "overwrite" replaces conflicting entities; "rename" creates with -N suffix;
// "cancel" is a no-op returning Skipped=true.
func (s *WorkflowExportService) Import(projectID string, req *types.ImportRequest) (*types.ImportResult, error) {
	action := req.Action
	if action != "overwrite" && action != "rename" && action != "cancel" {
		return nil, fmt.Errorf("invalid action %q: must be overwrite, rename, or cancel", action)
	}

	if action == "cancel" {
		return &types.ImportResult{
			WorkflowIDs:     []string{},
			PythonScriptIDs: []string{},
			Skipped:         true,
		}, nil
	}

	conflicts, err := s.CheckImport(projectID, &req.Bundle)
	if err != nil {
		return nil, err
	}

	conflictWF := make(map[string]bool, len(conflicts.WorkflowIDs))
	for _, id := range conflicts.WorkflowIDs {
		conflictWF[id] = true
	}
	conflictPS := make(map[string]bool, len(conflicts.PythonScriptIDs))
	for _, id := range conflicts.PythonScriptIDs {
		conflictPS[id] = true
	}

	if action == "overwrite" {
		for _, id := range conflicts.WorkflowIDs {
			if err := s.workflowSvc.DeleteWorkflowDef(projectID, id); err != nil && !strings.Contains(err.Error(), "not found") {
				return nil, fmt.Errorf("overwrite delete workflow %s: %w", id, err)
			}
		}
		for _, id := range conflicts.PythonScriptIDs {
			if err := s.pythonScriptSvc.Delete(projectID, id); err != nil && !strings.Contains(err.Error(), "not found") {
				return nil, fmt.Errorf("overwrite delete python script %s: %w", id, err)
			}
		}
	}

	// Create python scripts first; pythonScriptIDMap maps bundle ID → newly created ID.
	pythonScriptIDMap := map[string]string{}
	result := &types.ImportResult{
		WorkflowIDs:     []string{},
		PythonScriptIDs: []string{},
	}

	for _, script := range req.Bundle.PythonScripts {
		if script == nil {
			continue
		}
		var fp *string
		if script.FilePath != "" {
			v := script.FilePath
			fp = &v
		}
		created, err := s.pythonScriptSvc.Create(projectID, &types.PythonScriptCreateRequest{
			Name:            script.Name,
			Kind:            script.Kind,
			Description:     script.Description,
			Code:            script.Code,
			FilePath:        fp,
			ToolDescription: script.ToolDescription,
			InputSchema:     script.InputSchema,
			TimeoutSec:      script.TimeoutSec,
		})
		if err != nil {
			return nil, fmt.Errorf("create python script %s: %w", script.Name, err)
		}
		pythonScriptIDMap[script.ID] = created.ID
		result.PythonScriptIDs = append(result.PythonScriptIDs, created.ID)
	}

	for _, entry := range req.Bundle.Workflows {
		if entry.Workflow == nil {
			continue
		}

		finalID := entry.Workflow.ID
		if action == "rename" && conflictWF[entry.Workflow.ID] {
			finalID = s.findFreeWorkflowID(projectID, entry.Workflow.ID)
		}

		closeOnComplete := entry.Workflow.CloseTicketOnComplete
		wf, err := s.workflowSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
			ID:                    finalID,
			Description:           entry.Workflow.Description,
			ScopeType:             entry.Workflow.ScopeType,
			Groups:                entry.Workflow.GetGroups(),
			CloseTicketOnComplete: &closeOnComplete,
			NextWorkflowOnSuccess: entry.Workflow.NextWorkflowOnSuccess,
		})
		if err != nil {
			return nil, fmt.Errorf("create workflow %s: %w", finalID, err)
		}

		for _, agent := range entry.Agents {
			if agent == nil {
				continue
			}
			scriptID := agent.PythonScriptID
			if scriptID != nil {
				if mapped, ok := pythonScriptIDMap[*scriptID]; ok {
					scriptID = &mapped
				}
			}
			var validationCmds *[]string
			if agent.ValidationCommands != "" {
				var cmds []string
				if err := json.Unmarshal([]byte(agent.ValidationCommands), &cmds); err == nil {
					validationCmds = &cmds
				}
			}
			if _, err := s.agentDefSvc.CreateAgentDef(projectID, wf.ID, &types.AgentDefCreateRequest{
				ID:                     agent.ID,
				Model:                  agent.Model,
				Timeout:                agent.Timeout,
				Prompt:                 agent.Prompt,
				Layer:                  agent.Layer,
				RestartThreshold:       agent.RestartThreshold,
				MaxFailRestarts:        agent.MaxFailRestarts,
				StallStartTimeoutSec:   agent.StallStartTimeoutSec,
				StallRunningTimeoutSec: agent.StallRunningTimeoutSec,
				Tag:                    agent.Tag,
				LowConsumptionModel:    agent.LowConsumptionModel,
				ExecutionMode:          agent.ExecutionMode,
				Tools:                  agent.Tools,
				APIMaxIterations:       agent.APIMaxIterations,
				PythonScriptID:         scriptID,
				ValidationCommands:     validationCmds,
			}); err != nil {
				return nil, fmt.Errorf("create agent %s in workflow %s: %w", agent.ID, wf.ID, err)
			}
		}

		for layer, policy := range entry.LayerPolicies {
			if err := s.layerPolicySvc.SetLayerPolicy(projectID, wf.ID, layer, policy); err != nil {
				return nil, fmt.Errorf("set layer policy %d in workflow %s: %w", layer, wf.ID, err)
			}
		}

		for _, ch := range entry.Notifications {
			if ch == nil {
				continue
			}
			var configMap map[string]interface{}
			if ch.Config != "" {
				json.Unmarshal([]byte(ch.Config), &configMap) //nolint:errcheck
			}
			enabled := ch.Enabled
			msgTmpl := ch.MessageTemplate
			if _, err := s.notifySvc.Create(context.Background(), projectID, wf.ID, &types.NotificationChannelCreateRequest{
				Name:            ch.Name,
				Kind:            string(ch.Kind),
				Enabled:         &enabled,
				Config:          configMap,
				MessageTemplate: &msgTmpl,
				EventTypes:      ch.EventTypes,
			}); err != nil {
				return nil, fmt.Errorf("create notification channel %s: %w", ch.Name, err)
			}
		}

		result.WorkflowIDs = append(result.WorkflowIDs, wf.ID)
	}

	return result, nil
}

// findFreeWorkflowID returns the first available workflow ID by appending -N suffix.
func (s *WorkflowExportService) findFreeWorkflowID(projectID, base string) string {
	for i := 1; i <= 99; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		_, err := s.workflowSvc.GetWorkflowDef(projectID, candidate)
		if err != nil && strings.Contains(err.Error(), "not found") {
			return candidate
		}
	}
	return fmt.Sprintf("%s-imported", base)
}
