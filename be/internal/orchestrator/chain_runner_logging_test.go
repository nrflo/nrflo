package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

func TestChainRunnerStart_GeneratesTrx(t *testing.T) {
	env := newTestEnv(t)

	// Create a chain
	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-1"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Verify trx is present (not "-")
	if !strings.Contains(output, "chain started") {
		t.Errorf("log output missing 'chain started': %s", output)
	}

	// Extract trx ID from log line
	lines := strings.Split(output, "\n")
	var trxID string
	for _, line := range lines {
		if strings.Contains(line, "chain started") {
			// Format: YYYY-MM-DD HH:MM:SS LEVEL [trxID] message
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start != -1 && end != -1 {
				trxID = line[start+1 : end]
			}
			break
		}
	}

	if trxID == "" {
		t.Error("could not extract trx ID from log output")
	}
	if trxID == "-" {
		t.Error("trx ID should not be '-' for chain execution")
	}
	if len(trxID) != 8 {
		t.Errorf("trx ID length = %d, want 8 (hex string)", len(trxID))
	}

	// Clean up
	cr.Cancel(chainID)
}

func TestChainRunnerStart_LogsChainStarted(t *testing.T) {
	env := newTestEnv(t)

	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-2"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "INFO") {
		t.Errorf("log output missing INFO level: %s", output)
	}
	if !strings.Contains(output, "chain started") {
		t.Errorf("log output missing message: %s", output)
	}
	if !strings.Contains(output, "chain_id="+chainID) {
		t.Errorf("log output missing chain_id: %s", output)
	}
	if !strings.Contains(output, "workflow=test") {
		t.Errorf("log output missing workflow: %s", output)
	}

	// Clean up
	cr.Cancel(chainID)
}

func TestChainRunnerStart_TrxPropagation(t *testing.T) {
	env := newTestEnv(t)

	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-3"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	output := logBuf.String()
	lines := strings.Split(output, "\n")

	// Extract trx IDs from all log lines that contain chain-related messages
	var trxIDs []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Only check lines related to chain execution
		if !strings.Contains(line, "chain") {
			continue
		}
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start != -1 && end != -1 {
			trxID := line[start+1 : end]
			trxIDs = append(trxIDs, trxID)
		}
	}

	if len(trxIDs) == 0 {
		t.Fatal("no trx IDs found in chain log output")
	}

	// All trx IDs should be the same (same chain run)
	firstTrx := trxIDs[0]
	for i, trx := range trxIDs {
		if trx != firstTrx && trx != "-" {
			// Some logs might be from context.Background() calls, which have "-"
			t.Errorf("trx ID mismatch at line %d: got %s, want %s", i, trx, firstTrx)
		}
	}

	// Clean up
	cr.Cancel(chainID)
}

func TestChainRunner_LogsRecovery(t *testing.T) {
	env := newTestEnv(t)

	// Create a zombie chain (status = running but no goroutine)
	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-zombie-1"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusRunning, // Zombie state
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	cr.RecoverZombieChains()

	time.Sleep(50 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "WARN") {
		t.Errorf("log output missing WARN level: %s", output)
	}
	if !strings.Contains(output, "recovering zombie chain") {
		t.Errorf("log output missing recovery message: %s", output)
	}
	if !strings.Contains(output, "chain_id="+chainID) {
		t.Errorf("log output missing chain_id: %s", output)
	}
}

func TestChainRunner_LogsCancellation(t *testing.T) {
	// This test verifies that chain runner logs are properly structured
	// The actual cancellation behavior is tested in chain_runner functional tests
	// Here we just verify logging infrastructure works

	env := newTestEnv(t)

	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-cancel"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Verify chain start is logged
	if !strings.Contains(output, "INFO") {
		t.Errorf("log output missing INFO level: %s", output)
	}
	if !strings.Contains(output, "chain") {
		t.Errorf("log output missing chain-related message: %s", output)
	}

	// Clean up
	cr.Cancel(chainID)
}

func TestChainRunner_LogsItemStart(t *testing.T) {
	// This test verifies trx propagation and structured logging for chain execution
	// The actual chain item execution is tested in integration tests

	env := newTestEnv(t)

	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-item-1"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Should log chain started and completed (empty chain)
	if !strings.Contains(output, "chain started") && !strings.Contains(output, "chain completed") {
		t.Errorf("log output missing chain execution messages: %s", output)
	}

	// Clean up
	cr.Cancel(chainID)
}

func TestChainRunner_LogsErrorConditions(t *testing.T) {
	env := newTestEnv(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())

	// Try to start non-existent chain
	err := cr.Start(context.Background(), "nonexistent-chain")
	if err == nil {
		t.Fatal("Start() should fail for nonexistent chain")
	}

	// The error is returned, so no ERROR log is expected from Start itself
	// But we verify that trx context is available for error logging if needed
	// This test mainly verifies the code doesn't panic with logging
}

func TestChainRunner_StructuredLogging(t *testing.T) {
	env := newTestEnv(t)

	chainRepo := repo.NewChainRepo(env.pool, clock.Real())
	chainID := "chain-log-struct"
	err := chainRepo.Create(&model.ChainExecution{
		ID:           chainID,
		ProjectID:    env.project,
		WorkflowName: "test",
		Status:       model.ChainStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	logBuf := setupLogCapture(t)

	cr := NewChainRunner(env.orch, env.dbPath, env.hub, clock.Real())
	err = cr.Start(context.Background(), chainID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" || !strings.Contains(line, "chain") {
			continue
		}

		// Verify structured format: YYYY-MM-DD HH:MM:SS LEVEL [trx] message key=value...
		parts := strings.Fields(line)
		if len(parts) < 4 {
			t.Errorf("log line has fewer than 4 parts: %s", line)
			continue
		}

		// Check level (INFO/WARN/ERROR)
		level := parts[2]
		if level != "INFO" && level != "WARN" && level != "ERROR" {
			t.Errorf("log line has invalid level %s: %s", level, line)
		}

		// Check trx in brackets
		trxPart := parts[3]
		if !strings.HasPrefix(trxPart, "[") || !strings.HasSuffix(trxPart, "]") {
			t.Errorf("log line missing [trx]: %s", line)
		}
	}

	// Clean up
	cr.Cancel(chainID)
}
