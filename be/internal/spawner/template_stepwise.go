package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/service"
	"be/internal/service/stepengine"
)

// isStepwiseDef is the ONE place PromptMode is compared against
// service.PromptModeStepwise (Rule 6) — every other call site (loadTemplate,
// prepareSpawn, fetchPreviousDataAndReason, context_save.go) goes through
// this or stepwiseDefFor.
func isStepwiseDef(def *model.AgentDefinition) bool {
	return def != nil && def.PromptMode == service.PromptModeStepwise && def.Steps != nil && *def.Steps != ""
}

// stepwiseDefFor loads an agent def and reports whether it is stepwise.
func (s *Spawner) stepwiseDefFor(agentType, projectID, workflowName string) bool {
	return isStepwiseDef(s.loadAgentDefinition(agentType, projectID, workflowName))
}

// snapshotStepCursor idempotently creates the step cursor for a stepwise def
// before the prompt is assembled (prepareSpawn calls this before
// loadTemplate). Nil-safe: no pool, non-stepwise def, or an engine error all
// degrade silently — an unsnapshottable cursor falls back to rendering
// def.Steps directly (appendStepwiseBlock's no-cursor path).
func (s *Spawner) snapshotStepCursor(ctx context.Context, def *model.AgentDefinition, wfiID, nodeID string) {
	if !isStepwiseDef(def) {
		return
	}
	pool := s.pool()
	if pool == nil || wfiID == "" {
		return
	}
	engine := stepengine.New(pool, s.config.Clock, nil)
	if _, err := engine.Snapshot(wfiID, nodeID, def); err != nil {
		logger.Warn(ctx, "stepwise: snapshot cursor failed, will fall back to def steps", "error", err, "wfi_id", wfiID, "node_id", nodeID)
	}
}

// appendStepwiseBlock is a no-op (returns body unchanged) for full-mode/nil
// defs. For a stepwise def it appends the guidance injectable + a
// titles-only step outline ("step N of M") + the current step's full
// instruction. Step source precedence: the server-owned cursor snapshot
// (stepengine.Engine.State) when one exists — authoritative, immutable —
// falling back to decoding def.Steps directly only when there is no cursor
// (Preview, nodeID=="", pool-less). A future step's Instruction is never
// rendered.
func (s *Spawner) appendStepwiseBlock(body string, def *model.AgentDefinition, wfiID, nodeID string, vars map[string]string) string {
	if !isStepwiseDef(def) {
		return body
	}

	steps, currentIndex, completedIDs, revision := s.resolveStepwiseState(def, wfiID, nodeID)
	if len(steps) == 0 || currentIndex >= len(steps) {
		return body
	}
	current := steps[currentIndex]

	stepVars := make(map[string]string, len(vars)+5)
	for k, v := range vars {
		stepVars[k] = v
	}
	stepVars["STEP_INDEX"] = strconv.Itoa(currentIndex + 1)
	stepVars["STEP_TOTAL"] = strconv.Itoa(len(steps))
	stepVars["STEP_TITLE"] = current.Title
	stepVars["STEP_ID"] = current.StepID
	stepVars["STEP_REVISION"] = strconv.Itoa(revision)

	guidance := s.expandInjectable("stepwise-guidance", stepVars)

	var b strings.Builder
	if guidance != "" {
		b.WriteString("\n\n")
		b.WriteString(guidance)
	}
	fmt.Fprintf(&b, "\n\n## Steps (step %d of %d)\n", currentIndex+1, len(steps))
	for i, st := range steps {
		status := "locked"
		if completedIDs[st.StepID] {
			status = "completed"
		} else if i == currentIndex {
			status = "current"
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, status, st.Title)
	}
	fmt.Fprintf(&b, "\n### Current step: %s\n", current.Title)
	fmt.Fprintf(&b, "step_id=%s revision=%s\n\n", current.StepID, stepVars["STEP_REVISION"])
	b.WriteString(current.Instruction)

	return body + b.String()
}

// resolveStepwiseState returns (steps, currentIndex, completedStepIDs,
// revision) for a stepwise def: the cursor snapshot when one exists
// (authoritative), else def.Steps decoded directly with currentIndex 0,
// nothing completed, and revision 1 (Preview / nodeID=="" / pool-less
// fallback).
func (s *Spawner) resolveStepwiseState(def *model.AgentDefinition, wfiID, nodeID string) ([]model.StepDefinition, int, map[string]bool, int) {
	pool := s.pool()
	if pool != nil && wfiID != "" && nodeID != "" {
		engine := stepengine.New(pool, s.config.Clock, nil)
		if state, err := engine.State(wfiID, nodeID); err == nil {
			completed := make(map[string]bool, len(state.Completed))
			for _, c := range state.Completed {
				completed[c.StepID] = true
			}
			return state.Steps, state.CurrentIndex, completed, state.Revision
		}
	}

	var steps []model.StepDefinition
	if def.Steps != nil {
		if err := json.Unmarshal([]byte(*def.Steps), &steps); err != nil {
			return nil, 0, nil, 1
		}
	}
	return steps, 0, nil, 1
}
