package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
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
	clock        clock.Clock
}

// NewChainRunner creates a new chain runner
func NewChainRunner(orch *Orchestrator, dataPath string, wsHub *ws.Hub, clk clock.Clock) *ChainRunner {
	return &ChainRunner{
		runs:         make(map[string]context.CancelFunc),
		orchestrator: orch,
		dataPath:     dataPath,
		wsHub:        wsHub,
		clock:        clk,
	}
}

// Start begins sequential execution of a chain.
func (cr *ChainRunner) Start(ctx context.Context, chainID string) error {
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool, cr.clock)
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

	// Generate trx for this chain run — detach from HTTP request context
	// so the background goroutine isn't cancelled when the response is sent.
	trx := logger.NewTrx()
	chainCtx := logger.WithTrx(context.Background(), trx)

	runCtx, cancel := context.WithCancel(chainCtx)
	cr.runs[chainID] = cancel
	cr.mu.Unlock()

	logger.Info(runCtx, "chain started", "chain_id", chainID, "workflow", chain.WorkflowName)

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

		chainRepo := repo.NewChainRepo(pool, cr.clock)
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

// WaitAll blocks until all running chain goroutines have exited, or until timeout.
// Used in tests to avoid log buffer pollution between test runs.
func (cr *ChainRunner) WaitAll(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cr.mu.Lock()
		n := len(cr.runs)
		cr.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// RecoverZombieChains marks running chains as failed on startup (crash recovery)
func (cr *ChainRunner) RecoverZombieChains() {
	ctx := context.Background()
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		logger.Error(ctx, "failed to open DB for chain recovery", "err", err)
		return
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool, cr.clock)
	lockRepo := repo.NewChainLockRepo(pool)

	rows, err := pool.Query(`SELECT id, project_id FROM chain_executions WHERE status = 'running'`)
	if err != nil {
		logger.Error(ctx, "failed to query zombie chains", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, projectID string
		if err := rows.Scan(&id, &projectID); err != nil {
			continue
		}
		logger.Warn(ctx, "recovering zombie chain", "chain_id", id)
		chainRepo.UpdateStatus(id, model.ChainStatusFailed)
		lockRepo.DeleteLocksByChain(id)

		itemRepo := repo.NewChainItemRepo(pool, cr.clock)
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

	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		logger.Error(ctx, "failed to open database", "chain_id", chainID, "err", err)
		cr.markChainFailed(chainID, projectID)
		return
	}
	defer pool.Close()

	itemRepo := repo.NewChainItemRepo(pool, cr.clock)

	for {
		select {
		case <-ctx.Done():
			logger.Warn(ctx, "chain cancelled", "chain_id", chainID)
			cr.handleCancel(pool, chainID, projectID, workflowName)
			return
		default:
		}

		item, err := itemRepo.GetNextPending(chainID)
		if err != nil {
			logger.Error(ctx, "error getting next chain item", "chain_id", chainID, "err", err)
			cr.markChainFailed(chainID, projectID)
			return
		}
		if item == nil {
			logger.Info(ctx, "chain completed", "chain_id", chainID)
			cr.markChainCompleted(pool, chainID, projectID)
			return
		}

		if err := itemRepo.UpdateItemStatus(item.ID, model.ChainItemRunning); err != nil {
			logger.Error(ctx, "failed to update chain item status", "item", item.ID, "err", err)
			cr.markChainFailed(chainID, projectID)
			return
		}

		logger.Info(ctx, "chain item started", "chain_id", chainID, "ticket", item.TicketID, "position", item.Position)
		cr.broadcastChainUpdate(projectID, chainID, "item_started")

		result, err := cr.orchestrator.Start(ctx, RunRequest{
			ProjectID:    projectID,
			TicketID:     item.TicketID,
			WorkflowName: workflowName,
		})
		if err != nil {
			logger.Error(ctx, "chain item workflow failed to start", "ticket", item.TicketID, "err", err)
			itemRepo.UpdateItemStatus(item.ID, model.ChainItemFailed)
			cr.markChainFailed(chainID, projectID)
			return
		}

		if err := itemRepo.SetWorkflowInstanceID(item.ID, result.InstanceID); err != nil {
			logger.Error(ctx, "failed to set workflow instance ID for chain item", "item", item.ID, "err", err)
		}

		completed, success := cr.pollWorkflowInstance(ctx, result.InstanceID)
		if !completed {
			logger.Warn(ctx, "chain cancelled during item", "chain_id", chainID, "ticket", item.TicketID)
			cr.handleCancel(pool, chainID, projectID, workflowName)
			return
		}

		if success {
			if err := itemRepo.UpdateItemStatus(item.ID, model.ChainItemCompleted); err != nil {
				logger.Error(ctx, "failed to update chain item status to completed", "item", item.ID, "err", err)
			}
			logger.Info(ctx, "chain item completed", "chain_id", chainID, "ticket", item.TicketID)
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
				wfiRepo := repo.NewWorkflowInstanceRepo(pool, cr.clock)
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
	chainRepo := repo.NewChainRepo(pool, cr.clock)
	itemRepo := repo.NewChainItemRepo(pool, cr.clock)
	lockRepo := repo.NewChainLockRepo(pool)

	items, _ := itemRepo.ListByChain(chainID)
	for _, item := range items {
		if item.Status == model.ChainItemRunning {
			cr.orchestrator.StopByTicket(projectID, item.TicketID, workflowName, "")
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
	chainRepo := repo.NewChainRepo(pool, cr.clock)
	lockRepo := repo.NewChainLockRepo(pool)

	chainRepo.UpdateStatus(chainID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chainID)
	cr.broadcastChainUpdate(projectID, chainID, "completed")

	// Auto-close epic ticket if set (best-effort)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		logger.Error(context.Background(), "failed to load chain for epic close", "chain_id", chainID, "err", err)
		return
	}
	if chain.EpicTicketID != "" {
		ticketService := service.NewTicketService(pool, cr.clock)
		reason := fmt.Sprintf("All epic tickets completed via chain '%s'", chain.Name)
		if err := ticketService.Close(projectID, chain.EpicTicketID, reason); err != nil {
			logger.Error(context.Background(), "failed to close epic", "epic", chain.EpicTicketID, "err", err)
		} else if cr.wsHub != nil {
			cr.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, projectID, chain.EpicTicketID, "", map[string]interface{}{
				"status": "closed",
			}))
		}
		// Best-effort: auto-close parent epic if the chain's epic is itself a child
		if epic, err := ticketService.TryCloseParentEpic(projectID, chain.EpicTicketID); err != nil {
			logger.Error(context.Background(), "failed to auto-close parent epic", "epic", chain.EpicTicketID, "err", err)
		} else if epic != nil && cr.wsHub != nil {
			cr.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, projectID, epic.ID, "", map[string]interface{}{"status": "closed"}))
		}
	}
}

func (cr *ChainRunner) markChainFailed(chainID, projectID string) {
	pool, err := db.NewPool(cr.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return
	}
	defer pool.Close()

	chainRepo := repo.NewChainRepo(pool, cr.clock)
	lockRepo := repo.NewChainLockRepo(pool)

	itemRepo := repo.NewChainItemRepo(pool, cr.clock)
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
		cr.wsHub.Broadcast(ws.NewEvent(ws.EventChainUpdated, projectID, "", "", map[string]interface{}{
			"chain_id": chainID,
			"status":   status,
		}))
	}
}
