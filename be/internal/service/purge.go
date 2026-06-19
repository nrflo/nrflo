package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// PurgeCounts summarizes what a trace purge removed. It carries counts only — never any
// of the purged content — so it is safe to put in the audit log and a WS payload.
type PurgeCounts struct {
	SessionsRedacted int64 `json:"sessions_redacted"`
	MessagesDeleted  int64 `json:"messages_deleted"`
	FindingsDeleted  int64 `json:"findings_deleted"`
	HistoryDeleted   int64 `json:"findings_history_deleted"`
	ArtifactsDeleted int64 `json:"artifacts_deleted"`
	ErrorsDeleted    int64 `json:"errors_deleted"`
	FilesDeleted     int   `json:"artifact_files_deleted"`
	FileErrors       int   `json:"artifact_file_errors"`
}

// PurgeService scrubs a finished workflow instance's sensitive trace data, leaving a redacted,
// bare-minimal record. Driven by the per-workflow purge_on_completion flag (snapshotted onto the
// instance at creation) and invoked from the orchestrator's terminal-state hooks.
type PurgeService struct {
	pool     *db.Pool
	clock    clock.Clock
	hub      WSHub
	dataPath string
}

// NewPurgeService constructs a PurgeService. dataPath is the DB path (used to resolve the
// per-project artifact storage backend), matching NewArtifactService.
func NewPurgeService(pool *db.Pool, clk clock.Clock, hub WSHub, dataPath string) *PurgeService {
	return &PurgeService{pool: pool, clock: clk, hub: hub, dataPath: dataPath}
}

// PurgeInstanceTraceIfEnabled purges the instance's trace iff its purge_on_completion snapshot is
// set; otherwise it is a no-op. Idempotent: all operations are by-id UPDATE/DELETE, so a repeat
// call (or a re-run after a partial failure) is safe.
func (s *PurgeService) PurgeInstanceTraceIfEnabled(ctx context.Context, wfiID string) (*PurgeCounts, error) {
	wi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(wfiID)
	if err != nil {
		return nil, fmt.Errorf("purge: load instance: %w", err)
	}
	if !wi.PurgeOnCompletion {
		return nil, nil
	}
	return s.purge(ctx, wi)
}

func (s *PurgeService) purge(ctx context.Context, wi *model.WorkflowInstance) (*PurgeCounts, error) {
	counts := &PurgeCounts{}

	// 1. Delete artifact files from the storage backend first (best-effort; the DB rows that
	//    name them are deleted in the transaction below). A file that fails to delete is logged
	//    and counted, but does not abort the DB purge — leaving the browsable trace intact would
	//    be worse than an orphaned (logged) file.
	s.deleteArtifactFiles(ctx, wi, counts)

	// 2. Atomically redact the kept sessions and delete the remaining sensitive rows.
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	err := db.WithBusyRetry(func() error {
		tx, err := s.pool.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck // no-op once Commit succeeds
		if counts.SessionsRedacted, err = repo.RedactAgentSessions(tx, wi.ID, now); err != nil {
			return err
		}
		if counts.MessagesDeleted, err = repo.DeleteAgentMessagesByWorkflowInstance(tx, wi.ID); err != nil {
			return err
		}
		if counts.FindingsDeleted, err = repo.DeleteFindingsByWorkflowInstance(tx, wi.ID); err != nil {
			return err
		}
		if counts.HistoryDeleted, err = repo.DeleteFindingsHistoryByWorkflowInstance(tx, wi.ID); err != nil {
			return err
		}
		if counts.ArtifactsDeleted, err = repo.DeleteArtifactsByWorkflowInstance(tx, wi.ID); err != nil {
			return err
		}
		if counts.ErrorsDeleted, err = repo.DeleteErrorsByInstance(tx, wi.ID); err != nil {
			return err
		}
		if err = repo.ClearWorkflowInstanceCallerData(tx, wi.ID, now); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return counts, fmt.Errorf("purge: db: %w", err)
	}

	// 3. Record a content-free audit entry and tell the UI to refresh the now-empty panels.
	s.writeAudit(ctx, wi, counts)
	BroadcastFromCtx(s.hub, ws.EventWorkflowPurged,
		BroadcastCtx{ProjectID: wi.ProjectID, TicketID: wi.TicketID, Workflow: wi.WorkflowID},
		map[string]interface{}{"instance_id": wi.ID, "counts": counts})
	logger.Info(ctx, "purged workflow trace", "instance_id", wi.ID,
		"sessions", counts.SessionsRedacted, "messages", counts.MessagesDeleted,
		"findings", counts.FindingsDeleted, "artifacts", counts.ArtifactsDeleted,
		"errors", counts.ErrorsDeleted, "file_errors", counts.FileErrors)
	return counts, nil
}

func (s *PurgeService) deleteArtifactFiles(ctx context.Context, wi *model.WorkflowInstance, counts *PurgeCounts) {
	arts, err := repo.NewArtifactRepo(s.pool, s.clock).List(wi.ID)
	if err != nil {
		logger.Error(ctx, "purge: list artifacts", "instance_id", wi.ID, "err", err)
		return
	}
	if len(arts) == 0 {
		return
	}
	storage, err := NewArtifactService(s.pool, s.clock, s.hub, s.dataPath).GetStorage(ctx, wi.ProjectID)
	if err != nil {
		// Cannot reach the backend: leave the files (logged) rather than fail the whole purge.
		logger.Error(ctx, "purge: get storage (artifact files left on disk)", "instance_id", wi.ID, "err", err)
		counts.FileErrors = len(arts)
		return
	}
	for _, a := range arts {
		if err := storage.Delete(ctx, a.PathKey); err != nil {
			logger.Error(ctx, "purge: delete artifact file", "instance_id", wi.ID, "path_key", a.PathKey, "err", err)
			counts.FileErrors++
			continue
		}
		counts.FilesDeleted++
	}
}

func (s *PurgeService) writeAudit(ctx context.Context, wi *model.WorkflowInstance, counts *PurgeCounts) {
	meta, _ := json.Marshal(counts)
	entry := &model.AuditEntry{
		ID:           uuid.New().String(),
		Action:       "workflow.purged",
		ResourceType: "workflow_instance",
		ResourceID:   wi.ID,
		Metadata:     string(meta),
	}
	if err := repo.NewAuditRepo(s.pool, s.clock).Append(entry); err != nil {
		logger.Error(ctx, "purge: write audit", "instance_id", wi.ID, "err", err)
	}
}
