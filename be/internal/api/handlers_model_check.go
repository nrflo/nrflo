package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/service"
)

type modelTestResult struct {
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

type modelTestRequest struct {
	Mode string `json:"mode"`
}

// handleTestModel spawns a minimal CLI process to verify a model's CLI mode.
func (s *Server) handleTestModel(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" && r.ContentLength != 0 {
		var req modelTestRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		mode = req.Mode
	}
	if mode == "api" {
		writeError(w, http.StatusBadRequest, "api model testing is not supported")
		return
	}
	if mode != "" && mode != "cli" {
		writeError(w, http.StatusBadRequest, "mode must be cli")
		return
	}

	id := r.PathValue("id")
	m, err := service.NewModelService(s.pool, s.clock).Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if m.CLIModel == "" {
		writeError(w, http.StatusBadRequest, "model does not support cli mode")
		return
	}

	cliType := ""
	switch m.Provider {
	case "anthropic":
		cliType = "claude"
	case "openai":
		cliType = "codex"
	default:
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("unsupported model provider: %s", m.Provider))
		return
	}
	buildCmd := s.cliAdapterFunc
	if buildCmd == nil {
		buildCmd = buildModelCheckCommand
	}

	cmd, usesStdin := buildCmd(cliType, m.CLIModel, m.DefaultEffort)
	if cmd == nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("unsupported cli type: %s", cliType))
		return
	}
	cmd.Dir = os.TempDir()
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if usesStdin {
		cmd.Stdin = strings.NewReader("Reply with exactly: NRFLO_CHECK_OK")
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()

	start := time.Now()
	if err = cmd.Start(); err != nil {
		elapsed := time.Since(start).Milliseconds()
		logger.Warn(r.Context(), "model check start failed", "model", id, "error", err)
		writeJSON(w, http.StatusOK, &modelTestResult{
			Error: fmt.Sprintf("failed to start %s: %s", cliType, err), DurationMs: elapsed,
		})
		return
	}

	logger.Info(r.Context(), "model check started", "model", id, "cli_type", cliType, "cmd", strings.Join(cmd.Args, " "))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	elapsed := time.Since(start).Milliseconds()

	if ctx.Err() != nil {
		logger.Warn(r.Context(), "model check timeout", "model", id, "cli_type", cliType, "elapsed_ms", elapsed)
		writeJSON(w, http.StatusOK, &modelTestResult{
			Error:      fmt.Sprintf("$ %s\ntimeout after 40s waiting for %s to respond", strings.Join(cmd.Args, " "), cliType),
			DurationMs: elapsed,
		})
		return
	}
	if err != nil {
		errMsg := strings.TrimSpace(output.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		logger.Warn(r.Context(), "model check failed", "model", id, "elapsed_ms", elapsed, "error", errMsg)
		writeJSON(w, http.StatusOK, &modelTestResult{
			Error: fmt.Sprintf("$ %s\n%s", strings.Join(cmd.Args, " "), errMsg), DurationMs: elapsed,
		})
		return
	}

	logger.Info(r.Context(), "model check passed", "model", id, "elapsed_ms", elapsed)
	writeJSON(w, http.StatusOK, &modelTestResult{Success: true, DurationMs: elapsed})
}

// buildModelCheckCommand returns the batch command for a one-shot model check
// and whether to pipe the prompt via stdin.
func buildModelCheckCommand(cliType, mappedModel, reasoningEffort string) (*exec.Cmd, bool) {
	switch cliType {
	case "claude":
		return exec.Command("claude", "--print", "--verbose", "--dangerously-skip-permissions",
			"--output-format", "stream-json", "--model", mappedModel), true
	case "codex":
		args := []string{"exec", "--json", "--model", mappedModel, "--dangerously-bypass-approvals-and-sandbox"}
		if reasoningEffort != "" {
			args = append(args, "-c", fmt.Sprintf(`model_reasoning_effort="%s"`, reasoningEffort))
		}
		return exec.Command("codex", args...), true
	default:
		return nil, false
	}
}
