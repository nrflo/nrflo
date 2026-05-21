package orchestrator

import (
	"context"
	"os"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
)

type finalizeOutcome int

const (
	outcomeSuccess finalizeOutcome = iota
	outcomeFailure
)

// runFinalize executes the outcome-selected finalize slot after a workflow reaches terminal status.
// It never changes workflow status. Returns silently if no slot is configured.
func (o *Orchestrator) runFinalize(ctx context.Context, wfiID string, req RunRequest, outcome finalizeOutcome, detail string) {
	var cmd, scriptID, slot string
	if outcome == outcomeSuccess {
		cmd, scriptID, slot = req.FinalizeSuccessCommand, req.FinalizeSuccessScriptID, "success"
	} else {
		cmd, scriptID, slot = req.FinalizeFailureCommand, req.FinalizeFailureScriptID, "failure"
	}
	if cmd == "" && scriptID == "" {
		return
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "finalize: open db", "err", err)
		return
	}
	defer database.Close()
	pool := db.WrapAsPool(database)

	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(req.ProjectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		logger.Error(ctx, "finalize: project root missing", "project_id", req.ProjectID, "err", err)
		return
	}
	projectRoot := project.RootPath.String

	projectEnv := loadProjectEnv(ctx, pool, req.ProjectID, o.clock)
	env := makeFinalizeEnv(projectEnv, outcome, detail)

	ctx5s, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	var exitCode int
	var status, outputTail, kind, target string

	if cmd != "" {
		kind, target = "command", cmd
		exitCode, status, outputTail = runHookCommand(ctx5s, cmd, projectRoot, env)
	} else {
		kind, target = "script", scriptID
		exitCode, status, outputTail = o.runHookScript(ctx5s, pool, req.ProjectID, req.TicketID, projectRoot, "_finalize", scriptID, wfiID, env)
	}

	persistFinalizeFinding(o, pool, wfiID, req, slot, kind, target, exitCode, status, outputTail)
}

func makeFinalizeEnv(projectEnv []string, outcome finalizeOutcome, detail string) []string {
	env := os.Environ()
	env = append(env, projectEnv...)
	if outcome == outcomeSuccess {
		env = append(env,
			"NRF_WORKFLOW_STATUS=completed",
			"NRF_WORKFLOW_RESULT=pass",
			"NRF_WORKFLOW_FINAL_RESULT="+detail,
		)
	} else {
		env = append(env,
			"NRF_WORKFLOW_STATUS=failed",
			"NRF_WORKFLOW_RESULT=fail",
			"NRF_FAILURE_REASON="+detail,
		)
	}
	return env
}
