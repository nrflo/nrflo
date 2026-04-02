package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// attemptConflictResolution tries to resolve a merge conflict by spawning the
// conflict-resolver system agent. Returns nil on success (branch merged and
// deleted), or an error if resolution failed or no resolver is configured.
func (o *Orchestrator) attemptConflictResolution(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	wt *worktreeInfo,
	pool *db.Pool,
	mergeError string,
	modelConfigs map[string]spawner.ModelConfig,
	claudeSettingsJSON string,
) error {
	// Load conflict-resolver system agent definition
	svc := service.NewSystemAgentDefinitionService(pool, o.clock)
	sysDef, err := svc.Get("conflict-resolver")
	if err != nil {
		return fmt.Errorf("no conflict-resolver configured: %w", err)
	}

	// Broadcast resolving event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictResolving, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"branch":      wt.branchName,
		"merge_error": mergeError,
	}))

	// Construct spawner with synthetic single-phase workflow
	sp := spawner.New(spawner.Config{
		Workflows: map[string]spawner.WorkflowDef{
			"_conflict_resolution": {
				Phases: []spawner.PhaseDef{{ID: "conflict-resolver", Agent: "conflict-resolver", Layer: 0}},
			},
		},
		Agents: map[string]spawner.AgentConfig{
			"conflict-resolver": {Model: sysDef.Model, Timeout: sysDef.Timeout},
		},
		DataPath:           o.dataPath,
		ProjectRoot:        wt.projectRoot,
		WSHub:              o.wsHub,
		Pool:               pool,
		Clock:              o.clock,
		ClaudeSettingsJSON: claudeSettingsJSON,
		ModelConfigs:       modelConfigs,
		ErrorSvc:           o.errorSvc,
	})

	spawnErr := sp.Spawn(ctx, spawner.SpawnRequest{
		AgentType:          "conflict-resolver",
		TicketID:           req.TicketID,
		ProjectID:          req.ProjectID,
		WorkflowName:       "_conflict_resolution",
		WorkflowInstanceID: wfiID,
		ScopeType:          req.ScopeType,
		ExtraVars: map[string]string{
			"BRANCH_NAME":    wt.branchName,
			"DEFAULT_BRANCH": wt.defaultBranch,
			"MERGE_ERROR":    mergeError,
		},
	})
	sp.Close()

	if spawnErr != nil {
		if o.errorSvc != nil {
			o.errorSvc.RecordError(req.ProjectID, "system", wfiID, fmt.Sprintf("merge conflict resolution failed for branch %s: %s", wt.branchName, spawnErr.Error()))
		}
		o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"branch":      wt.branchName,
			"error":       spawnErr.Error(),
		}))
		return fmt.Errorf("conflict resolution failed: %w", spawnErr)
	}

	// Resolution succeeded — delete the feature branch
	cmd := exec.Command("git", "branch", "-d", wt.branchName)
	cmd.Dir = wt.projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(ctx, "failed to delete branch after conflict resolution", "branch", wt.branchName, "err", err, "output", strings.TrimSpace(string(out)))
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictResolved, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"branch":      wt.branchName,
	}))

	return nil
}
