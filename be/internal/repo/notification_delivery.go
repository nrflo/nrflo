package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"

	"github.com/google/uuid"
)

// NotificationDeliveryRepo handles notification_deliveries CRUD.
type NotificationDeliveryRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewNotificationDeliveryRepo creates a new NotificationDeliveryRepo.
func NewNotificationDeliveryRepo(database db.Querier, clk clock.Clock) *NotificationDeliveryRepo {
	return &NotificationDeliveryRepo{db: database, clock: clk}
}

const notificationDeliveryCols = `id, channel_id, project_id, event_type, payload, status, attempts, last_error, next_attempt_at, created_at, updated_at`

func (r *NotificationDeliveryRepo) scanRow(row interface{ Scan(...interface{}) error }) (*model.NotificationDelivery, error) {
	var id, channelID, projectID, eventType, payload, status, lastError, createdAt, updatedAt string
	var attempts int
	var nextAttemptAt sql.NullString

	if err := row.Scan(&id, &channelID, &projectID, &eventType, &payload, &status, &attempts, &lastError, &nextAttemptAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	d := &model.NotificationDelivery{
		ID:        id,
		ChannelID: channelID,
		ProjectID: projectID,
		EventType: eventType,
		Payload:   payload,
		Status:    model.DeliveryStatus(status),
		Attempts:  attempts,
		LastError: lastError,
	}

	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	if nextAttemptAt.Valid && nextAttemptAt.String != "" {
		ts, err := time.Parse(time.RFC3339Nano, nextAttemptAt.String)
		if err == nil {
			d.NextAttemptAt = &ts
		}
	}

	return d, nil
}

// Insert creates a new delivery record.
func (r *NotificationDeliveryRepo) Insert(d *model.NotificationDelivery) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	d.ID = uuid.New().String()
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	d.UpdatedAt = d.CreatedAt

	_, err := r.db.Exec(
		`INSERT INTO notification_deliveries (`+notificationDeliveryCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(d.ID),
		strings.ToLower(d.ChannelID),
		strings.ToLower(d.ProjectID),
		d.EventType,
		d.Payload,
		string(d.Status),
		d.Attempts,
		d.LastError,
		nullableTime(d.NextAttemptAt),
		now,
		now,
	)
	return err
}

// ListPending returns pending deliveries whose next_attempt_at is <= now or null, oldest first.
func (r *NotificationDeliveryRepo) ListPending(now time.Time, limit int) ([]*model.NotificationDelivery, error) {
	nowStr := now.UTC().Format(time.RFC3339Nano)
	rows, err := r.db.Query(
		`SELECT `+notificationDeliveryCols+` FROM notification_deliveries
		 WHERE status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		 ORDER BY created_at ASC LIMIT ?`,
		nowStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*model.NotificationDelivery
	for rows.Next() {
		d, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// UpdateStatus updates the status, attempts, last error, and next_attempt_at for a delivery.
func (r *NotificationDeliveryRepo) UpdateStatus(id string, status model.DeliveryStatus, attempts int, lastError string, nextAttemptAt *time.Time) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE notification_deliveries SET status=?, attempts=?, last_error=?, next_attempt_at=?, updated_at=? WHERE LOWER(id) = LOWER(?)`,
		string(status),
		attempts,
		lastError,
		nullableTime(nextAttemptAt),
		now,
		id,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("notification delivery not found: %s", id)
	}
	return nil
}

// ListByChannel returns deliveries for a channel, newest first.
func (r *NotificationDeliveryRepo) ListByChannel(channelID string, limit int) ([]*model.NotificationDelivery, error) {
	rows, err := r.db.Query(
		`SELECT `+notificationDeliveryCols+` FROM notification_deliveries WHERE LOWER(channel_id) = LOWER(?) ORDER BY created_at DESC LIMIT ?`,
		channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*model.NotificationDelivery
	for rows.Next() {
		d, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}
