package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"be/internal/service"
	"be/internal/spawner"

	"github.com/google/uuid"
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

	getAdapter := s.cliAdapterFunc
	if getAdapter == nil {
		getAdapter = spawner.GetCLIAdapter
	}
	adapter, err := getAdapter(m.CLIType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cli binary not found: %s", m.CLIType))
		return
	}

	prompt := "Reply with exactly: NRFLOW_CHECK_OK"

	opts := spawner.SpawnOptions{
		Model:           m.ID,
		MappedModel:     m.MappedModel,
		ReasoningEffort: m.ReasoningEffort,
		SessionID:       uuid.New().String(),
		WorkDir:         os.TempDir(),
		Prompt:          prompt,
		Env:             os.Environ(),
	}

	cmd := adapter.BuildCommand(opts)

	if adapter.UsesStdinPrompt() {
		cmd.Stdin = strings.NewReader(prompt)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	start := time.Now()
	err = cmd.Start()
	if err != nil {
		elapsed := time.Since(start).Milliseconds()
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to start %s: %s", m.CLIType, err.Error()),
			DurationMs: elapsed,
		})
		return
	}

	// BuildCommand uses exec.Command (no context), so we manually kill on timeout.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err = <-done:
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done // wait for Wait() to release resources
	}
	elapsed := time.Since(start).Milliseconds()

	if ctx.Err() != nil {
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      fmt.Sprintf("timeout after 60s waiting for %s to respond", m.CLIType),
			DurationMs: elapsed,
		})
		return
	}

	if err != nil {
		errMsg := strings.TrimSpace(output.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
			Success:    false,
			Error:      errMsg,
			DurationMs: elapsed,
		})
		return
	}

	writeJSON(w, http.StatusOK, &service.TestCLIModelResult{
		Success:    true,
		DurationMs: elapsed,
	})
}
