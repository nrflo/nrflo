package repo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"

	"github.com/google/uuid"
)

// NotificationChannelRepo handles notification_channels CRUD.
type NotificationChannelRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewNotificationChannelRepo creates a new NotificationChannelRepo.
func NewNotificationChannelRepo(database db.Querier, clk clock.Clock) *NotificationChannelRepo {
	return &NotificationChannelRepo{db: database, clock: clk}
}

const notificationChannelCols = `id, project_id, workflow_id, name, kind, enabled, config, event_types, created_at, updated_at`

func (r *NotificationChannelRepo) scanRow(row interface{ Scan(...interface{}) error }) (*model.NotificationChannel, error) {
	var id, projectID, workflowID, name, kind, config, eventTypesJSON, createdAt, updatedAt string
	var enabled int

	if err := row.Scan(&id, &projectID, &workflowID, &name, &kind, &enabled, &config, &eventTypesJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	ch := &model.NotificationChannel{
		ID:         id,
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Name:       name,
		Kind:       model.ChannelKind(kind),
		Enabled:    enabled != 0,
		Config:     config,
	}

	if err := json.Unmarshal([]byte(eventTypesJSON), &ch.EventTypes); err != nil {
		ch.EventTypes = []string{}
	}

	ch.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	ch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return ch, nil
}

// Insert creates a new notification channel.
func (r *NotificationChannelRepo) Insert(ch *model.NotificationChannel) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	ch.ID = uuid.New().String()
	ch.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	ch.UpdatedAt = ch.CreatedAt

	etJSON, err := json.Marshal(ch.EventTypes)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(
		`INSERT INTO notification_channels (`+notificationChannelCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(ch.ID),
		strings.ToLower(ch.ProjectID),
		strings.ToLower(ch.WorkflowID),
		ch.Name,
		string(ch.Kind),
		boolToInt(ch.Enabled),
		ch.Config,
		string(etJSON),
		now,
		now,
	)
	return err
}

// Get retrieves a channel by ID.
func (r *NotificationChannelRepo) Get(id string) (*model.NotificationChannel, error) {
	row := r.db.QueryRow(
		`SELECT `+notificationChannelCols+` FROM notification_channels WHERE LOWER(id) = LOWER(?)`, id)
	ch, err := r.scanRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("notification channel not found: %s", id)
	}
	return ch, err
}

// Update persists all fields on ch (full replace).
func (r *NotificationChannelRepo) Update(ch *model.NotificationChannel) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)

	etJSON, err := json.Marshal(ch.EventTypes)
	if err != nil {
		return err
	}

	result, err := r.db.Exec(
		`UPDATE notification_channels SET name=?, kind=?, enabled=?, config=?, event_types=?, updated_at=? WHERE LOWER(id) = LOWER(?)`,
		ch.Name,
		string(ch.Kind),
		boolToInt(ch.Enabled),
		ch.Config,
		string(etJSON),
		now,
		ch.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("notification channel not found: %s", ch.ID)
	}
	ch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return nil
}

// Delete removes a channel by ID.
func (r *NotificationChannelRepo) Delete(id string) error {
	result, err := r.db.Exec(`DELETE FROM notification_channels WHERE LOWER(id) = LOWER(?)`, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("notification channel not found: %s", id)
	}
	return nil
}

// ListByWorkflow returns all channels for a project+workflow.
func (r *NotificationChannelRepo) ListByWorkflow(projectID, workflowID string) ([]*model.NotificationChannel, error) {
	rows, err := r.db.Query(
		`SELECT `+notificationChannelCols+` FROM notification_channels WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) ORDER BY created_at ASC`,
		projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.NotificationChannel
	for rows.Next() {
		ch, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// ListEnabledForEvent returns enabled channels for a project+workflow that subscribe to eventType.
func (r *NotificationChannelRepo) ListEnabledForEvent(projectID, workflowID, eventType string) ([]*model.NotificationChannel, error) {
	// Use JSON LIKE matching: event_types is stored as JSON array ["a","b","c"]
	// Match substring `"eventType"` within the JSON.
	pattern := `%"` + eventType + `"%`
	rows, err := r.db.Query(
		`SELECT `+notificationChannelCols+` FROM notification_channels WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND enabled = 1 AND event_types LIKE ?`,
		projectID, workflowID, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.NotificationChannel
	for rows.Next() {
		ch, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}
