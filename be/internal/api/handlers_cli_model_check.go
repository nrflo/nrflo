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

// handleTestCLIModel spawns a minimal agent process to verify a CLI model config works.
func (s *Server) handleTestCLIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewCLIModelService(s.pool, s.clock)

	m, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	buildCmd := s.cliAdapterFunc
	if buildCmd == nil {
		buildCmd = buildModelCheckCommand
	}

	prompt := "Reply with exactly: NRFLO_CHECK_OK"

	cmd, usesStdin := buildCmd(m.CLIType, m.MappedModel, m.ReasoningEffort)
	if cmd == nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("unsupported cli type: %s", m.CLIType))
		return
	}
	cmd.Dir = os.TempDir()
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if usesStdin {
		cmd.Stdin = strings.NewReader(prompt)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()

	start := time.Now()
	err = cmd.Start()
	if err != nil {
		elapsed := time.Since(start).Milliseconds()
		logger.Warn(r.Context(), "cli model check start failed", "model", id, "error", err)
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to start %s: %s", m.CLIType, err.Error()),
			DurationMs: elapsed,
		})
		return
	}

	logger.Info(r.Context(), "cli model check started", "model", id, "cli_type", m.CLIType, "cmd", strings.Join(cmd.Args, " "))

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err = <-done:
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	elapsed := time.Since(start).Milliseconds()

	if ctx.Err() != nil {
		logger.Warn(r.Context(), "cli model check timeout", "model", id, "cli_type", m.CLIType, "elapsed_ms", elapsed, "output", strings.TrimSpace(output.String()))
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      fmt.Sprintf("$ %s\ntimeout after 40s waiting for %s to respond", strings.Join(cmd.Args, " "), m.CLIType),
			DurationMs: elapsed,
		})
		return
	}

	if err != nil {
		errMsg := strings.TrimSpace(output.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		logger.Warn(r.Context(), "cli model check failed", "model", id, "elapsed_ms", elapsed, "error", errMsg)
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      fmt.Sprintf("$ %s\n%s", strings.Join(cmd.Args, " "), errMsg),
			DurationMs: elapsed,
		})
		return
	}

	logger.Info(r.Context(), "cli model check passed", "model", id, "elapsed_ms", elapsed)
	writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
		Success:    true,
		DurationMs: elapsed,
	})
}

// buildModelCheckCommand returns the batch command for a one-shot model check
// and whether to pipe the prompt via stdin (true) or argv (false).
func buildModelCheckCommand(cliType, mappedModel, reasoningEffort string) (*exec.Cmd, bool) {
	switch cliType {
	case "claude":
		args := []string{
			"--print",
			"--verbose",
			"--dangerously-skip-permissions",
			"--output-format", "stream-json",
			"--model", mappedModel,
		}
		return exec.Command("claude", args...), true
	case "codex":
		args := []string{
			"exec",
			"--json",
			"--model", mappedModel,
			"--dangerously-bypass-approvals-and-sandbox",
		}
		if reasoningEffort != "" {
			args = append(args, "-c", fmt.Sprintf(`model_reasoning_effort="%s"`, reasoningEffort))
		}
		return exec.Command("codex", args...), true
	case "opencode":
		args := []string{
			"run",
			"--format", "json",
			"--model", mappedModel,
			"Reply with exactly: NRFLO_CHECK_OK",
		}
		return exec.Command("opencode", args...), false
	default:
		return nil, false
	}
}
