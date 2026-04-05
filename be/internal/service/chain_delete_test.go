package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// insertChain inserts a raw chain_executions row for unit testing.
func insertChain(t *testing.T, svc *ChainService, projectID, chainID string, status model.ChainStatus) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := svc.pool.Exec(`
		INSERT INTO chain_executions (id, project_id, name, status, workflow_name, created_by, created_at, updated_at)
		VALUES (?, ?, 'Test Chain', ?, 'feature', 'test', ?, ?)`,
		chainID, projectID, string(status), now, now,
	); err != nil {
		t.Fatalf("insert chain %q: %v", chainID, err)
	}
}

// TestDeleteChain_DeletableStatuses verifies pending/completed/failed/canceled can be deleted.
func TestDeleteChain_DeletableStatuses(t *testing.T) {
	cases := []model.ChainStatus{
		model.ChainStatusPending,
		model.ChainStatusCompleted,
		model.ChainStatusFailed,
		model.ChainStatusCanceled,
	}
	for _, status := range cases {
		t.Run(string(status), func(t *testing.T) {
			pool, projectID := setupChainTestDB(t)
			defer pool.Close()

			svc := NewChainService(pool, clock.Real())
			chainID := "chain-" + string(status)
			insertChain(t, svc, projectID, chainID, status)

			if err := svc.DeleteChain(projectID, chainID); err != nil {
				t.Errorf("DeleteChain(%q, %q) = %v, want nil", projectID, chainID, err)
			}
		})
	}
}

// TestDeleteChain_RunningRejected verifies running chains cannot be deleted.
func TestDeleteChain_RunningRejected(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool, clock.Real())
	insertChain(t, svc, projectID, "chain-running", model.ChainStatusRunning)

	err := svc.DeleteChain(projectID, "chain-running")
	if err == nil {
		t.Fatal("DeleteChain(running) = nil, want error")
	}
	if !strings.Contains(err.Error(), "running") {
		t.Errorf("error = %q, want to contain 'running'", err.Error())
	}
}

// TestDeleteChain_NotFound verifies error when chain does not exist.
func TestDeleteChain_NotFound(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool, clock.Real())

	err := svc.DeleteChain(projectID, "no-such-chain")
	if err == nil {
		t.Fatal("DeleteChain(missing) = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// TestDeleteChain_ProjectMismatch verifies error when chain belongs to a different project.
func TestDeleteChain_ProjectMismatch(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool, clock.Real())
	insertChain(t, svc, projectID, "chain-pm", model.ChainStatusCompleted)

	err := svc.DeleteChain("wrong-project", "chain-pm")
	if err == nil {
		t.Fatal("DeleteChain(wrong project) = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// TestDeleteChain_ChainGoneAfterDelete verifies chain is no longer retrievable after deletion.
func TestDeleteChain_ChainGoneAfterDelete(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool, clock.Real())
	insertChain(t, svc, projectID, "chain-gone", model.ChainStatusCompleted)

	if err := svc.DeleteChain(projectID, "chain-gone"); err != nil {
		t.Fatalf("DeleteChain = %v, want nil", err)
	}

	// Second delete must also fail with "not found".
	err := svc.DeleteChain(projectID, "chain-gone")
	if err == nil {
		t.Fatal("second DeleteChain = nil, want error (chain gone)")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}
