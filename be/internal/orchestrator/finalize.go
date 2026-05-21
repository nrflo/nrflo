package orchestrator

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

type finalizeOutcome int

const (
	outcomeSuccess finalizeOutcome = iota
	outcomeFailure
)

const (
	finalizeTimeout   = 5 * time.Second
	finalizeOutputCap = 2048
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

	// Resolve original project root (not worktree path).
	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(req.ProjectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		logger.Error(ctx, "finalize: project root missing", "project_id", req.ProjectID, "err", err)
		return
	}
	projectRoot := project.RootPath.String

	projectEnv := loadProjectEnv(ctx, pool, req.ProjectID, o.clock)
	env := makeFinalizeEnv(projectEnv, outcome, detail)

	ctx5s, cancel := context.WithTimeout(ctx, finalizeTimeout)
	defer cancel()

	var exitCode int
	var status, outputTail, kind, target string

	if cmd != "" {
		kind, target = "command", cmd
		exitCode, status, outputTail = runFinalizeCommand(ctx5s, cmd, projectRoot, env)
	} else {
		kind, target = "script", scriptID
		exitCode, status, outputTail = o.runFinalizeScript(ctx5s, pool, req.ProjectID, req.TicketID, projectRoot, scriptID, wfiID, env)
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

func runFinalizeCommand(ctx context.Context, cmd, dir string, env []string) (int, string, string) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = dir
	c.Env = env
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	out := tailFinalizeOutput(buf.String())
	if err == nil {
		return 0, "ok", out
	}
	if ctx.Err() == context.DeadlineExceeded {
		return -1, "timeout", out
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), "failed", out
	}
	return -1, "failed", out
}

func (o *Orchestrator) runFinalizeScript(ctx context.Context, pool *db.Pool, projectID, ticketID, projectRoot, scriptID, wfiID string, env []string) (int, string, string) {
	scriptRepo := repo.NewPythonScriptRepo(pool, o.clock)
	script, err := scriptRepo.Get(projectID, scriptID)
	if err != nil {
		return -1, "failed", fmt.Sprintf("script not found: %v", err)
	}

	scriptCode := script.Code
	if script.FilePath != "" {
		if !filepath.IsAbs(script.FilePath) {
			return -1, "failed", "file_path must be absolute"
		}
		info, statErr := os.Stat(script.FilePath)
		if statErr != nil {
			return -1, "failed", fmt.Sprintf("stat file_path: %v", statErr)
		}
		if !info.Mode().IsRegular() || !strings.HasSuffix(script.FilePath, ".py") {
			return -1, "failed", "file_path must be a regular .py file"
		}
		data, readErr := os.ReadFile(script.FilePath)
		if readErr != nil {
			return -1, "failed", fmt.Sprintf("read file_path: %v", readErr)
		}
		scriptCode = string(data)
	}

	if err := os.MkdirAll("/tmp/nrflo/finalize", 0o755); err != nil {
		return -1, "failed", fmt.Sprintf("mkdir: %v", err)
	}
	sid := uuid.New().String()
	scriptPath := fmt.Sprintf("/tmp/nrflo/finalize/%s.py", sid)
	if err := os.WriteFile(scriptPath, []byte(scriptCode), 0o600); err != nil {
		return -1, "failed", fmt.Sprintf("write script: %v", err)
	}
	defer os.Remove(scriptPath)

	token := spawner.MintSpawnToken()
	now := o.clock.Now().UTC().Format(time.RFC3339Nano)
	sess := &model.AgentSession{
		ID:                 sid,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		AgentType:          "_finalize",
		Status:             model.AgentSessionRunning,
		SpawnToken:         sql.NullString{String: token, Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	sessRepo := repo.NewAgentSessionRepo(pool, o.clock)
	if err := sessRepo.Create(sess); err != nil {
		return -1, "failed", fmt.Sprintf("create session: %v", err)
	}

	pythonBin := "python3"
	if o.venvMgr != nil && projectRoot != "" {
		if bin, venvErr := o.venvMgr.Ensure(ctx, projectID, projectRoot); venvErr == nil && bin != "" {
			pythonBin = bin
		}
	}

	sessEnv := append(env,
		"NRFLO_PROJECT="+projectID,
		"NRF_WORKFLOW_INSTANCE_ID="+wfiID,
		"NRF_SESSION_ID="+sid,
		"NRFLO_AGENT_TOKEN="+token,
		"NRF_SPAWNED=1",
	)
	if o.sdkDir != "" {
		sessEnv = append(sessEnv, "NRFLO_SDK_DIR="+o.sdkDir)
	}

	c := exec.CommandContext(ctx, pythonBin, scriptPath)
	c.Dir = projectRoot
	c.Env = sessEnv
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	runErr := c.Run()
	out := tailFinalizeOutput(buf.String())

	if runErr == nil {
		_ = sessRepo.UpdateStatusEnded(sid, model.AgentSessionCompleted)
		return 0, "ok", out
	}
	_ = sessRepo.UpdateStatusToFailedWithReason(sid, fmt.Sprintf("finalize script failed: %v", runErr))
	if ctx.Err() == context.DeadlineExceeded {
		return -1, "timeout", out
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), "failed", out
	}
	return -1, "failed", out
}

func tailFinalizeOutput(s string) string {
	if len(s) > finalizeOutputCap {
		return s[len(s)-finalizeOutputCap:]
	}
	return s
}
