package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// prepareScriptSpawn handles execution_mode="script" agent prep:
// loads the python_scripts row, builds minimal SpawnOptions with NRF_* env, and
// returns early without template loading or CLI adapter resolution.
func (s *Spawner) prepareScriptSpawn(ctx context.Context, req SpawnRequest, phase, wfiID, agentID, sessionID, spawnToken string, agentDef *model.AgentDefinition) (*processInfo, *prepResult, error) {
	if s.config.PythonScriptRepo == nil {
		return nil, nil, fmt.Errorf("python_script_id_required: PythonScriptRepo not configured")
	}
	if agentDef == nil || agentDef.PythonScriptID == nil {
		return nil, nil, fmt.Errorf("python_script_id_required")
	}

	script, err := s.config.PythonScriptRepo.Get(req.ProjectID, *agentDef.PythonScriptID)
	if err != nil {
		return nil, nil, fmt.Errorf("python_script_not_found: %w", err)
	}

	scriptCode := script.Code
	if script.FilePath != "" {
		if !filepath.IsAbs(script.FilePath) {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must be absolute")
		}
		info, err := os.Stat(script.FilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must be a regular file")
		}
		if !strings.HasSuffix(script.FilePath, ".py") {
			return nil, nil, fmt.Errorf("python_script_file_path_invalid: file_path must end in .py")
		}
		data, err := os.ReadFile(script.FilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("python_script_file_path_read: %w", err)
		}
		scriptCode = string(data)
	}

	// Resolve timeout from agent config or agent definition.
	timeout := 40
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.Timeout > 0 {
		timeout = agentCfg.Timeout
	}
	if agentDef.Timeout > 0 {
		timeout = agentDef.Timeout
	}

	// Stall settings: stall_start disabled by default for scripts.
	stallStartTimeout := time.Duration(0)
	stallRunningTimeout := defaultStallRunningTimeout
	if agentDef.StallStartTimeoutSec != nil {
		if *agentDef.StallStartTimeoutSec == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
		}
	}
	if agentDef.StallRunningTimeoutSec != nil {
		if *agentDef.StallRunningTimeoutSec == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
		}
	}

	var scriptValidationCommands []string
	if agentDef.ValidationCommands != "" {
		if jsonErr := json.Unmarshal([]byte(agentDef.ValidationCommands), &scriptValidationCommands); jsonErr != nil {
			logger.Warn(ctx, "failed to parse validation_commands", "agent", req.AgentType, "error", jsonErr)
			scriptValidationCommands = nil
		}
	}

	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	modelID := "script:" + script.ID

	scriptStageDir, _ := EnsureStageDir(req.ProjectID, wfiID)
	if s.config.ArtifactSvc != nil {
		if pool := s.pool(); pool != nil {
			if storage, storageErr := s.config.ArtifactSvc.GetStorage(ctx, req.ProjectID); storageErr == nil {
				if _, matErr := MaterializeAll(ctx, wfiID, req.ProjectID, repo.NewArtifactRepo(pool, s.config.Clock), storage); matErr != nil {
					logger.Warn(ctx, "artifact pre-materialize failed during script spawn", "error", matErr)
				}
			} else {
				logger.Warn(ctx, "artifact storage unavailable during script spawn", "error", storageErr)
			}
		}
	}

	extID, extCtx, _ := s.fetchExternalRefs(req.ProjectID, req.TicketID, req.WorkflowName, wfiID)
	env := append(HostEnvWithoutClaudeMarkers(),
		fmt.Sprintf("NRFLO_PROJECT=%s", req.ProjectID),
		fmt.Sprintf("NRF_WORKFLOW_INSTANCE_ID=%s", wfiID),
		fmt.Sprintf("NRF_SESSION_ID=%s", sessionID),
		fmt.Sprintf("NRFLO_AGENT_TOKEN=%s", spawnToken),
		fmt.Sprintf("NRF_TRX=%s", logger.TrxFromContext(ctx)),
		"NRF_SPAWNED=1",
		fmt.Sprintf("NRF_ARTIFACTS_DIR=%s", scriptStageDir),
		fmt.Sprintf("NRF_EXTERNAL_ID=%s", extID),
		fmt.Sprintf("NRF_EXTERNAL_CONTEXT=%s", extCtx),
	)
	env = append(env, s.config.ProjectEnv...)

	proc := &processInfo{
		agentID:             agentID,
		agentType:           req.AgentType,
		nodeID:              phase,
		modelID:             modelID,
		sessionID:           sessionID,
		spawnToken:          spawnToken,
		startTime:           s.config.Clock.Now(),
		timeout:             time.Duration(timeout) * time.Minute,
		pendingMessages:     make([]repo.MessageEntry, 0),
		pendingTasks:        make(map[string]taskInfo),
		doneCh:              make(chan struct{}),
		sessionStartCh:      make(chan struct{}),
		firstByteCh:         make(chan struct{}),
		lastMessagesFlush:   s.config.Clock.Now(),
		projectID:           req.ProjectID,
		ticketID:            req.TicketID,
		workflowName:        req.WorkflowName,
		workflowInstanceID:  wfiID,
		lastMessageTime:     s.config.Clock.Now(),
		stallStartTimeout:   stallStartTimeout,
		stallRunningTimeout: stallRunningTimeout,
		maxContext:          0,
		restartThreshold:    defaultContextThreshold,
		validationCommands:  scriptValidationCommands,
		workDir:             workDir,
		env:                 env,
	}
	logger.Info(ctx, "agent spawned with validation commands", "agent", req.AgentType, "count", len(proc.validationCommands))

	prep := &prepResult{
		executionMode: "script",
		scriptCode:    scriptCode,
		scriptID:      script.ID,
		pythonPath:    s.config.PythonPath,
		phase:         phase,
		nodeID:        phase,
		opts: SpawnOptions{
			WorkDir: workDir,
			Env:     env,
		},
	}

	return proc, prep, nil
}
