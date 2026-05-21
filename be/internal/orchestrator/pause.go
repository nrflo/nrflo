package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// maybePauseAfterLayer checks whether completedLayerIdx has pause_after=true and, if so,
// fires the pause hook, persists a _pause finding, sets instance status to waiting, and
// broadcasts EventWorkflowPaused. Returns true if the workflow is now paused.
// The caller must set worktreeHandled=true and return immediately when this returns true.
func (o *Orchestrator) maybePauseAfterLayer(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	completedLayerIdx int,
	layerGroups []layerGroup,
	layerPause map[int]bool,
	pool *db.Pool,
	projectRoot string,
) bool {
	if completedLayerIdx < 0 || completedLayerIdx+1 >= len(layerGroups) {
		return false
	}
	completedLayer := layerGroups[completedLayerIdx].layer
	if !layerPause[completedLayer] {
		return false
	}
	nextLayer := layerGroups[completedLayerIdx+1].layer

	logger.Info(ctx, "pausing workflow after layer", "layer", completedLayer, "next_layer", nextLayer, "instance_id", wfiID)

	cmd := req.PauseEventCommand
	scriptID := req.PauseEventScriptID

	env := os.Environ()
	env = append(env,
		"NRF_WORKFLOW_STATUS=waiting",
		"NRF_PAUSED_AFTER_LAYER="+fmt.Sprintf("%d", completedLayer),
		"NRF_NEXT_LAYER="+fmt.Sprintf("%d", nextLayer),
		"NRF_WORKFLOW_INSTANCE_ID="+wfiID,
	)
	env = append(env, loadProjectEnv(ctx, pool, req.ProjectID, o.clock)...)

	ctx5s, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	var exitCode int
	var status, outputTail, kind, target string
	if cmd != "" {
		kind, target = "command", cmd
		exitCode, status, outputTail = runHookCommand(ctx5s, cmd, projectRoot, env)
	} else if scriptID != "" {
		kind, target = "script", scriptID
		exitCode, status, outputTail = o.runHookScript(ctx5s, pool, req.ProjectID, req.TicketID, projectRoot, "_pause", scriptID, wfiID, env)
	}

	persistPauseFinding(o, pool, wfiID, req, completedLayer, nextLayer, kind, target, exitCode, status, outputTail)

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "pause: open db", "err", err)
		return true
	}
	defer database.Close()
	wfiRepo := repo.NewWorkflowInstanceRepo(db.WrapAsPool(database), o.clock)
	_ = wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceWaiting)

	o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowPaused, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":        wfiID,
		"paused_after_layer": completedLayer,
		"resume_layer":       nextLayer,
	}))

	return true
}

// persistPauseFinding upserts the _pause finding for a workflow instance.
func persistPauseFinding(o *Orchestrator, pool *db.Pool, wfiID string, req RunRequest, pausedAfterLayer, resumeLayer int, kind, target string, exitCode int, status, outputTail string) {
	event := map[string]interface{}{
		"kind":        kind,
		"target":      target,
		"exit_code":   exitCode,
		"status":      status,
		"output_tail": outputTail,
	}
	val, _ := json.Marshal(map[string]interface{}{
		"paused_after_layer": pausedAfterLayer,
		"resume_layer":       resumeLayer,
		"event":              event,
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
	})
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	_ = findingRepo.Upsert("workflow_instance", wfiID, "_pause", val,
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"})
}
