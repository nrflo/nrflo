package api

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
)

func (s *Server) initEventLog() {
	elRepo := repo.NewEventLogRepo(s.pool, s.clock)
	s.wsHub.SetEventLog(elRepo)

	// Set up snapshot provider backed by WorkflowService
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	s.wsHub.SetSnapshotProvider(service.NewWorkflowSnapshotProvider(wfSvc))

	// Start retention cleanup: delete events older than 24h, every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			deleted, err := elRepo.Cleanup(24 * time.Hour)
			if err != nil {
				logger.Info(context.Background(), "event log cleanup error", "error", err)
			} else if deleted > 0 {
				logger.Info(context.Background(), "event log cleanup", "deleted", deleted)
			}
		}
	}()
}

// startRetentionCleanup runs per-project workflow instance cleanup every 20 minutes.
// For each project with workflow_cleanup_enabled=true and a per-project retention_limit
// set, non-active instances beyond that limit are deleted. If no per-project limit is
// configured (sentinel 0), the project is skipped.
// Orphaned messages are purged once per pass after all projects are processed.
func (s *Server) startRetentionCleanup() {
	cleanup := func() {
		svc := service.NewGlobalSettingsService(s.pool, s.clock)
		projectRepo := repo.NewProjectRepo(s.pool, s.clock)
		wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
		asRepo := repo.NewAgentSessionRepo(s.pool, s.clock)

		projects, err := projectRepo.List()
		if err != nil {
			logger.Info(context.Background(), "retention cleanup: list projects error", "error", err)
		} else {
			for _, project := range projects {
				enabled, err := svc.GetWorkflowCleanupEnabled(project.ID)
				if err != nil {
					logger.Info(context.Background(), "retention cleanup: get cleanup enabled error", "project", project.ID, "error", err)
					continue
				}
				if !enabled {
					continue
				}
				keep, err := svc.GetSessionRetentionLimit(project.ID)
				if err != nil {
					logger.Info(context.Background(), "retention cleanup: get retention limit error", "project", project.ID, "error", err)
					continue
				}
				if keep <= 0 {
					logger.Info(context.Background(), "retention cleanup: limit not configured", "project", project.ID)
					continue
				}
				if deleted, err := wfiRepo.CleanupKeepLatestForProject(project.ID, keep); err != nil {
					logger.Info(context.Background(), "retention cleanup: workflow_instances error", "project", project.ID, "error", err)
				} else if deleted > 0 {
					logger.Info(context.Background(), "retention cleanup: workflow_instances", "project", project.ID, "deleted", deleted)
				}
			}
		}

		// Clean up orphaned messages (sessions removed by CASCADE).
		if deleted, err := asRepo.CleanupOrphanedMessages(); err != nil {
			logger.Info(context.Background(), "retention cleanup: orphaned messages error", "error", err)
		} else if deleted > 0 {
			logger.Info(context.Background(), "retention cleanup: orphaned messages", "deleted", deleted)
		}

		// Reap staged uploads older than 1 hour.
		s.reapStaleUploads()

		// Auto-cancel plan drafts idle past plan_draft_ttl_min.
		planSvc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
		if cancelled, err := planSvc.SweepExpiredDrafts(s.clock.Now()); err != nil {
			logger.Info(context.Background(), "retention cleanup: plan draft sweep error", "error", err)
		} else if cancelled > 0 {
			logger.Info(context.Background(), "retention cleanup: plan drafts cancelled", "count", cancelled)
		}

		// Expire console sessions idle past console_idle_ttl_hours.
		consoleSvc := service.NewConsoleService(s.pool, s.clock)
		if expired, err := consoleSvc.SweepIdle(s.clock.Now()); err != nil {
			logger.Info(context.Background(), "retention cleanup: console idle sweep error", "error", err)
		} else if expired > 0 {
			logger.Info(context.Background(), "retention cleanup: console sessions expired", "count", expired)
		}
	}

	// Run once immediately on startup
	go func() {
		cleanup()
		ticker := time.NewTicker(20 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanup()
		}
	}()
}

// reapStaleUploads removes staged upload directories older than 1 hour.
func (s *Server) reapStaleUploads() {
	if s.dataPath == "" {
		return
	}
	artifactSvc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	root := artifactSvc.StagingRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := s.clock.Now().Add(-1 * time.Hour)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rmErr := os.RemoveAll(filepath.Join(root, e.Name())); rmErr == nil {
				logger.Info(context.Background(), "retention cleanup: reaped stale upload", "upload_id", e.Name())
			}
		}
	}
}
