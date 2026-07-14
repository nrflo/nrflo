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
	"be/internal/id"
	"be/internal/model"
	"be/internal/repo"
)

const (
	hookTimeout   = 5 * time.Second
	hookOutputCap = 2048
)

// runHookCommand runs a shell command and returns (exitCode, status, outputTail).
func runHookCommand(ctx context.Context, cmd, dir string, env []string) (int, string, string) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = dir
	c.Env = env
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	out := tailOutput(buf.String())
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

// runHookScript runs a python_scripts row as a transient agent session and returns (exitCode, status, outputTail).
// agentType is used for the session record (e.g. "_finalize", "_pause").
func (o *Orchestrator) runHookScript(ctx context.Context, pool *db.Pool, projectID, ticketID, projectRoot, agentType, scriptID, wfiID string, env []string) (int, string, string) {
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

	dir := fmt.Sprintf("/tmp/nrflo/%s", agentType)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return -1, "failed", fmt.Sprintf("mkdir: %v", err)
	}
	sid := uuid.New().String()
	scriptPath := fmt.Sprintf("%s/%s.py", dir, sid)
	if err := os.WriteFile(scriptPath, []byte(scriptCode), 0o600); err != nil {
		return -1, "failed", fmt.Sprintf("write script: %v", err)
	}
	defer os.Remove(scriptPath)

	token := id.MintToken()
	now := o.clock.Now().UTC().Format(time.RFC3339Nano)
	sess := &model.AgentSession{
		ID:                 sid,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		AgentType:          agentType,
		NodeID:             agentType,
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
	out := tailOutput(buf.String())

	if runErr == nil {
		_ = sessRepo.UpdateStatusEnded(sid, model.AgentSessionCompleted)
		return 0, "ok", out
	}
	_ = sessRepo.UpdateStatusToFailedWithReason(sid, fmt.Sprintf("%s script failed: %v", agentType, runErr))
	if ctx.Err() == context.DeadlineExceeded {
		return -1, "timeout", out
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), "failed", out
	}
	return -1, "failed", out
}

func tailOutput(s string) string {
	if len(s) > hookOutputCap {
		return s[len(s)-hookOutputCap:]
	}
	return s
}
