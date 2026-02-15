package repo

import (
	"database/sql"
	"time"

	"be/internal/db"
	"be/internal/model"
)

type DailyStatsRepo struct {
	db *db.DB
}

func NewDailyStatsRepo(database *db.DB) *DailyStatsRepo {
	return &DailyStatsRepo{db: database}
}

// Upsert inserts or replaces daily stats for a given project and date.
func (r *DailyStatsRepo) Upsert(projectID, date string, stats model.DailyStats) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO daily_stats
			(project_id, date, tickets_created, tickets_closed, tokens_spent, agent_time_sec, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, date,
		stats.TicketsCreated, stats.TicketsClosed, stats.TokensSpent, stats.AgentTimeSec,
		now,
	)
	return err
}

// GetByDate returns daily stats for a project on a given date.
// Returns a zero-value struct if no row exists.
func (r *DailyStatsRepo) GetByDate(projectID, date string) (model.DailyStats, error) {
	var s model.DailyStats
	err := r.db.QueryRow(`
		SELECT id, project_id, date, tickets_created, tickets_closed, tokens_spent, agent_time_sec, updated_at
		FROM daily_stats
		WHERE project_id = ? AND date = ?`,
		projectID, date,
	).Scan(&s.ID, &s.ProjectID, &s.Date, &s.TicketsCreated, &s.TicketsClosed, &s.TokensSpent, &s.AgentTimeSec, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return model.DailyStats{}, nil
	}
	return s, err
}
