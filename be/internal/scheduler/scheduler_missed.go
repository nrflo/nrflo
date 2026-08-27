package scheduler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"be/internal/db"
	"be/internal/model"
)

const missedRunReason = "server_offline"

func (s *Scheduler) advancePastMissedRuns(task *model.ScheduledTask, sched cron.Schedule, now time.Time) (time.Time, int, error) {
	now = now.In(now.Location())
	next := sched.Next(now)
	if task.NextRunAt != nil {
		candidate := task.NextRunAt.In(now.Location())
		if scheduledTimeMatches(sched, candidate) {
			next = candidate
		} else if task.LastTriggeredAt != nil {
			next = sched.Next(task.LastTriggeredAt.In(now.Location()))
		}
	}

	skipped := 0
	for !next.After(now) {
		following := sched.Next(next)
		if err := s.insertSkippedRunAndAdvance(task, next, following); err != nil {
			return time.Time{}, skipped, fmt.Errorf("record skipped run for %s: %w", task.ID, err)
		}
		skipped++
		next = following
	}
	return next, skipped, nil
}

func (s *Scheduler) insertSkippedRunAndAdvance(task *model.ScheduledTask, scheduledAt, nextRun time.Time) error {
	return db.WithBusyRetry(func() error {
		tx, err := s.pool.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if _, err := tx.Exec(
			`INSERT INTO schedule_runs (id, scheduled_task_id, project_id, triggered_at, status, workflows, chain_runs, error)
			 VALUES (?, ?, ?, ?, 'skipped', '[]', '[]', ?)`,
			uuid.New().String(), task.ID, task.ProjectID,
			scheduledAt.UTC().Format(time.RFC3339Nano), missedRunReason,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE scheduled_tasks SET next_run_at=?, updated_at=? WHERE LOWER(id)=LOWER(?)`,
			nextRun.UTC().Format(time.RFC3339Nano), s.clock.Now().UTC().Format(time.RFC3339Nano), task.ID,
		); err != nil {
			return err
		}
		return tx.Commit()
	})
}
