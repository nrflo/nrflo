package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

const chainPollInterval = 5 * time.Second

// ChainRunner executes chain items sequentially using the orchestrator.
type ChainRunner struct {
	mu           sync.Mutex
	runs         map[string]context.CancelFunc // chainID → cancel
	orchestrator *Orchestrator
	dataPath     string
	wsHub        *ws.Hub
}

// NewChainRunner creates a new chain runner
func NewChainRunner(orch *Orchestrator, dataPath string, wsHub *ws.Hub) *ChainRunner {
	return &ChainRunner{
		runs:         make(map[string]context.CancelFunc),
		orchestrator: orch,
		dataPath:     dataPath,
		wsHub:        wsHub,
	}
}

// Start begins sequential execution of a chain.
func (cr *ChainRunner) Start(ctx context.Context, chainID string) error {
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		return err
	}
	if chain.Status != model.ChainStatusPending {
		return fmt.Errorf("chain must be in pending status to start (current: %s)", chain.Status)
	}

	// Check not already running and register atomically
	cr.mu.Lock()
	if _, running := cr.runs[chainID]; running {
		cr.mu.Unlock()
		return fmt.Errorf("chain %s is already running", chainID)
	}

	// Set chain to running in DB while holding the lock
	if err := chainRepo.UpdateStatus(chainID, model.ChainStatusRunning); err != nil {
		cr.mu.Unlock()
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	cr.runs[chainID] = cancel
	cr.mu.Unlock()

	cr.broadcastChainUpdate(chain.ProjectID, chainID, "running")

	go cr.runLoop(runCtx, chainID, chain.ProjectID, chain.WorkflowName)

	return nil
}

// Cancel cancels a running chain.
func (cr *ChainRunner) Cancel(chainID string) error {
	cr.mu.Lock()
	cancel, ok := cr.runs[chainID]
	cr.mu.Unlock()

	if !ok {
		// Not in runs map — try to cancel in DB directly (e.g., pending chain)
		pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
		if err != nil {
			return err
		}
		defer pool.Close()

		chainRepo := repo.NewChainRepo(pool)
		chain, err := chainRepo.Get(chainID)
		if err != nil {
			return err
		}
		if chain.Status == model.ChainStatusPending {
			if err := chainRepo.UpdateStatus(chainID, model.ChainStatusCanceled); err != nil {
				return err
			}
			lockRepo := repo.NewChainLockRepo(pool)
			lockRepo.DeleteLocksByChain(chainID)
			cr.broadcastChainUpdate(chain.ProjectID, chainID, "canceled")
			return nil
		}
		return fmt.Errorf("chain %s is not running", chainID)
	}

	cancel()
	return nil
}

// IsRunning checks if a chain is currently being executed
func (cr *ChainRunner) IsRunning(chainID string) bool {
	cr.mu.Lock()
	_, ok := cr.runs[chainID]
	cr.mu.Unlock()
	return ok
}

// RecoverZombieChains marks running chains as failed on startup (crash recovery)
func (cr *ChainRunner) RecoverZombieChains() {
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		log.Printf("[chain-runner] Failed to open DB for recovery: %v", err)
		return
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool)
	lockRepo := repo.NewChainLockRepo(pool)

	rows, err := pool.Query(`SELECT id, project_id FROM chain_executions WHERE status = 'running'`)
	if err != nil {
		log.Printf("[chain-runner] Failed to query zombie chains: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, projectID string
		if err := rows.Scan(&id, &projectID); err != nil {
			continue
		}
		log.Printf("[chain-runner] Recovering zombie chain %s (marking as failed)", id)
		chainRepo.UpdateStatus(id, model.ChainStatusFailed)
		lockRepo.DeleteLocksByChain(id)

		itemRepo := repo.NewChainItemRepo(pool)
		items, _ := itemRepo.ListByChain(id)
		for _, item := range items {
			if item.Status == model.ChainItemRunning || item.Status == model.ChainItemPending {
				itemRepo.UpdateItemStatus(item.ID, model.ChainItemCanceled)
			}
		}
	}
}

func (cr *ChainRunner) runLoop(ctx context.Context, chainID, projectID, workflowName string) {
	defer func() {
		cr.mu.Lock()
		delete(cr.runs, chainID)
		cr.mu.Unlock()
	}()

	log.Printf("[chain-runner] Starting chain %s", chainID)

	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		log.Printf("[chain-runner] Failed to open database: %v", err)
		cr.markChainFailed(chainID, projectID)
		return
	}
	defer pool.Close()

	itemRepo := repo.NewChainItemRepo(pool)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[chain-runner] Chain %s canceled", chainID)
			cr.handleCancel(pool, chainID, projectID, workflowName)
			return
		default:
		}

		item, err := itemRepo.GetNextPending(chainID)
		if err != nil {
			log.Printf("[chain-runner] Error getting next item: %v", err)
			cr.markChainFailed(chainID, projectID)
			return
		}
		if item == nil {
			log.Printf("[chain-runner] Chain %s completed", chainID)
			cr.markChainCompleted(pool, chainID, projectID)
			return
		}

		if err := itemRepo.UpdateItemStatus(item.ID, model.ChainItemRunning); err != nil {
			log.Printf("[chain-runner] Failed to update item status: %v", err)
			cr.markChainFailed(chainID, projectID)
			return
		}

		cr.broadcastChainUpdate(projectID, chainID, "item_started")

		result, err := cr.orchestrator.Start(ctx, RunRequest{
			ProjectID:    projectID,
			TicketID:     item.TicketID,
			WorkflowName: workflowName,
		})
		if err != nil {
			log.Printf("[chain-runner] Failed to start workflow for %s: %v", item.TicketID, err)
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemFailed)
			cr.markChainFailed(chainID, projectID)
			return
		}

		if err := itemRepo.SetWorkflowInstanceID(item.ID, result.InstanceID); err != nil {
			log.Printf("[chain-runner] Failed to set workflow instance ID for item %s: %v", item.ID, err)
		}

		completed, success := cr.pollWorkflowInstance(ctx, result.InstanceID)
		if !completed {
			log.Printf("[chain-runner] Chain %s canceled during item %s", chainID, item.TicketID)
			cr.handleCancel(pool, chainID, projectID, workflowName)
			return
		}

		if success {
			if err := itemRepo.UpdateItemStatus(item.ID, model.ChainItemCompleted); err != nil {
				log.Printf("[chain-runner] Failed to update item status to completed: %v", err)
			}
			cr.broadcastChainUpdate(projectID, chainID, "item_completed")
		} else {
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemFailed)
			cr.markChainFailed(chainID, projectID)
			return
		}
	}
}

// pollWorkflowInstance polls the workflow instance status until it finishes.
func (cr *ChainRunner) pollWorkflowInstance(ctx context.Context, instanceID string) (finished bool, success bool) {
	ticker := time.NewTicker(chainPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, false
		case <-ticker.C:
			if !cr.orchestrator.IsInstanceRunning(instanceID) {
				pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
				if err != nil {
					return true, false
				}
				wfiRepo := repo.NewWorkflowInstanceRepo(pool)
				wi, err := wfiRepo.Get(instanceID)
				pool.Close()
				if err != nil {
					return true, false
				}
				return true, wi.Status == model.WorkflowInstanceCompleted
			}
		}
	}
}

func (cr *ChainRunner) handleCancel(pool *db.Pool, chainID, projectID, workflowName string) {
	chainRepo := repo.NewChainRepo(pool)
	itemRepo := repo.NewChainItemRepo(pool)
	lockRepo := repo.NewChainLockRepo(pool)

	items, _ := itemRepo.ListByChain(chainID)
	for _, item := range items {
		if item.Status == model.ChainItemRunning {
			cr.orchestrator.StopByTicket(projectID, item.TicketID, workflowName)
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemCanceled)
		} else if item.Status == model.ChainItemPending {
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemCanceled)
		}
	}

	chainRepo.UpdateStatus(chainID, model.ChainStatusCanceled)
	lockRepo.DeleteLocksByChain(chainID)
	cr.broadcastChainUpdate(projectID, chainID, "canceled")
}

func (cr *ChainRunner) markChainCompleted(pool *db.Pool, chainID, projectID string) {
	chainRepo := repo.NewChainRepo(pool)
	lockRepo := repo.NewChainLockRepo(pool)

	chainRepo.UpdateStatus(chainID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chainID)
	cr.broadcastChainUpdate(projectID, chainID, "completed")

	// Auto-close epic ticket if set (best-effort)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		log.Printf("[chain-runner] Failed to load chain %s for epic close: %v", chainID, err)
		return
	}
	if chain.EpicTicketID != "" {
		ticketService := service.NewTicketService(pool)
		reason := fmt.Sprintf("All epic tickets completed via chain '%s'", chain.Name)
		if err := ticketService.Close(projectID, chain.EpicTicketID, reason); err != nil {
			log.Printf("[chain-runner] Failed to close epic %s: %v", chain.EpicTicketID, err)
		} else if cr.wsHub != nil {
			cr.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, projectID, chain.EpicTicketID, "", map[string]interface{}{
				"status": "closed",
			}))
		}
	}
}

func (cr *ChainRunner) markChainFailed(chainID, projectID string) {
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool)
	lockRepo := repo.NewChainLockRepo(pool)

	itemRepo := repo.NewChainItemRepo(pool)
	items, _ := itemRepo.ListByChain(chainID)
	for _, item := range items {
		if item.Status == model.ChainItemPending {
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemCanceled)
		}
	}

	chainRepo.UpdateStatus(chainID, model.ChainStatusFailed)
	lockRepo.DeleteLocksByChain(chainID)
	cr.broadcastChainUpdate(projectID, chainID, "failed")
}

func (cr *ChainRunner) broadcastChainUpdate(projectID, chainID, status string) {
	if cr.wsHub != nil {
		cr.wsHub.Broadcast(ws.NewEvent("chain.updated", projectID, "", "", map[string]interface{}{
			"chain_id": chainID,
			"status":   status,
		}))
	}
}
