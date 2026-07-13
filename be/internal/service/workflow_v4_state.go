package service

import (
	"encoding/json"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

// buildV4State builds the v4-compatible response from a workflow instance
func (s *WorkflowService) buildV4State(wi *model.WorkflowInstance) map[string]interface{} {
	scopeType := wi.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	// Load workflow definition for phase derivation
	var phaseOrder []string
	phases := make(map[string]model.PhaseStatus)
	currentPhase := ""
	var phaseLayers map[string]int

	// Materialized plan nodes (DYNWF-5) merge into the static phase graph so
	// the existing PhaseGraph renders dynamic layers with no separate read path.
	materializedPhases, materializedPolicies, _ := LoadInstanceNodePhases(s.pool, s.clock, wi.ID)

	if wf, err := s.GetWorkflowDef(wi.ProjectID, wi.WorkflowID); err == nil {
		allPhases := append([]PhaseDef{}, wf.Phases...)
		for _, p := range materializedPhases {
			allPhases = append(allPhases, PhaseDef{NodeID: p.NodeID, Agent: p.Agent, Layer: p.Layer})
		}
		phaseOrder = make([]string, len(allPhases))
		phaseLayers = make(map[string]int, len(allPhases))
		for i, p := range allPhases {
			phaseOrder[i] = p.NodeID
			phaseLayers[p.NodeID] = p.Layer
		}
		phases = s.derivePhaseStatuses(wi.ID, allPhases)
		currentPhase = s.deriveCurrentPhase(wi.ID)
	}

	result := map[string]interface{}{
		"version":        4,
		"initialized_at": wi.CreatedAt.Format(time.RFC3339Nano),
		"instance_id":    wi.ID,
		"scope_type":     scopeType,
		"current_phase":  currentPhase,
		"retry_count":    wi.RetryCount,
		"phases":         phases,
		"phase_order":    phaseOrder,
		"workflow":       wi.WorkflowID,
		"agent_retries":  map[string]int{},
	}
	if phaseLayers != nil {
		result["phase_layers"] = phaseLayers
	}
	// Include per-layer pass policies (def-scoped + materialized) so the UI can render policy badges
	policies, _ := NewWorkflowLayerPolicyService(s.pool, s.clock).GetLayerPolicies(wi.ProjectID, wi.WorkflowID)
	if len(materializedPolicies) > 0 {
		if policies == nil {
			policies = make(map[int]string, len(materializedPolicies))
		}
		for layer, policy := range materializedPolicies {
			policies[layer] = policy
		}
	}
	if len(policies) > 0 {
		result["layer_policies"] = policies
	}

	// Plan lifecycle block: derived read model for the async plan boundary caller.
	if head, err := repo.NewPlanRepo(s.pool, s.clock).GetHead(wi.ID); err == nil {
		planBlock := map[string]interface{}{
			"status":                head.Status,
			"latest_revision":       head.LatestRevision,
			"approved_revision":     head.ApprovedRevision,
			"materialized_revision": head.MaterializedRevision,
		}
		if head.LatestRevision > 0 {
			if rev, rerr := repo.NewPlanRepo(s.pool, s.clock).GetRevision(wi.ID, head.LatestRevision); rerr == nil {
				if m, perr := ParsePlanManifest(json.RawMessage(rev.Manifest)); perr == nil {
					planBlock["questions"] = m.Questions
				}
			}
		}
		result["plan"] = planBlock
	}
	if wi.ParentSession.Valid {
		result["parent_session"] = wi.ParentSession.String
	}
	if wi.WorktreePath.Valid {
		result["worktree_path"] = wi.WorktreePath.String
	}
	if wi.BranchName.Valid {
		result["branch_name"] = wi.BranchName.String
	}
	result["endless_loop"] = wi.EndlessLoop
	result["stop_endless_loop_after_iteration"] = wi.StopEndlessLoopAfterIteration

	// Completion stats
	result["status"] = string(wi.Status)
	if wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceProjectCompleted {
		result["completed_at"] = wi.UpdatedAt.Format(time.RFC3339Nano)
		result["total_duration_sec"] = wi.UpdatedAt.Sub(wi.CreatedAt).Seconds()
	}

	// Restart details (shared across active agents and history)
	detailsMap := s.loadRestartDetails(wi.ID)

	// Active agents from agent_sessions
	result["active_agents"] = s.buildActiveAgentsMap(wi.ID, detailsMap)

	// Agent history from completed sessions
	agentHistory := s.buildAgentHistory(wi.ID, detailsMap)
	result["agent_history"] = agentHistory

	// Total tokens used (per-session context window via cli_models)
	if wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceProjectCompleted {
		ctxLengths := s.loadModelContextLengths()
		var totalTokens int64
		for _, entry := range agentHistory {
			if m, ok := entry.(map[string]interface{}); ok {
				if cl, exists := m["context_left"]; exists {
					if contextLeft, ok := cl.(int64); ok {
						ctxLen := int64(200000)
						if modelID, ok := m["model_id"].(string); ok && modelID != "" {
							if l, found := ctxLengths[modelID]; found {
								ctxLen = l
							}
						}
						totalTokens += ctxLen * (100 - contextLeft) / 100
					}
				}
			}
		}
		result["total_tokens_used"] = totalTokens
	}

	// Combined findings: workflow-level + per-session
	result["findings"] = s.BuildCombinedFindings(wi)

	// Workflow-level findings from findings table (excluding internal keys starting with _)
	wfRaw, _ := s.findingRepo.GetOwn("workflow_instance", wi.ID)
	if len(wfRaw) > 0 {
		filtered := make(map[string]interface{})
		for k, v := range wfRaw {
			if !strings.HasPrefix(k, "_") {
				var parsed interface{}
				json.Unmarshal(v, &parsed) //nolint:errcheck
				filtered[k] = parsed
			}
		}
		if len(filtered) > 0 {
			result["workflow_findings"] = filtered
		}
		// Extract _callback as top-level field for frontend visualization
		if cbRaw, ok := wfRaw["_callback"]; ok {
			var cb map[string]interface{}
			if json.Unmarshal(cbRaw, &cb) == nil {
				result["callback"] = map[string]interface{}{
					"level":        cb["level"],
					"instructions": cb["instructions"],
					"from_layer":   cb["from_layer"],
					"from_agent":   cb["from_agent"],
				}
			}
		}
		if fRaw, ok := wfRaw["_finalize"]; ok {
			var fin map[string]interface{}
			if json.Unmarshal(fRaw, &fin) == nil {
				result["finalize_result"] = fin
			}
		}
		if pRaw, ok := wfRaw["_pause"]; ok {
			var pause map[string]interface{}
			if json.Unmarshal(pRaw, &pause) == nil {
				result["pause_result"] = pause
			}
		}
	}

	// Extract workflow_final_result from agent session findings
	if finalResult := s.ExtractWorkflowFinalResult(wi); finalResult != "" {
		result["workflow_final_result"] = finalResult
	}

	return result
}
