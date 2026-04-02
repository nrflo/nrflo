package service

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"

	"github.com/google/uuid"
)

// ErrorService provides error tracking operations.
type ErrorService struct {
	pool *db.Pool
	clock clock.Clock
	hub   *ws.Hub
}

// NewErrorService creates a new ErrorService.
func NewErrorService(pool *db.Pool, clk clock.Clock, hub *ws.Hub) *ErrorService {
	return &ErrorService{pool: pool, clock: clk, hub: hub}
}

// RecordError creates a new error record and broadcasts a WS event.
func (s *ErrorService) RecordError(projectID, errorType, instanceID, message string) error {
	e := &model.ErrorLog{
		ID:         uuid.New().String(),
		ProjectID:  projectID,
		ErrorType:  model.ErrorType(errorType),
		InstanceID: instanceID,
		Message:    message,
		CreatedAt:  s.clock.Now().UTC().Format(time.RFC3339Nano),
	}

	r := repo.NewErrorLogRepo(s.pool, s.clock)
	if err := r.Insert(e); err != nil {
		return fmt.Errorf("record error: %w", err)
	}

	if s.hub != nil {
		s.hub.Broadcast(ws.NewEvent(ws.EventErrorCreated, projectID, "", "", map[string]interface{}{
			"id":          e.ID,
			"error_type":  string(e.ErrorType),
			"instance_id": e.InstanceID,
			"message":     e.Message,
			"created_at":  e.CreatedAt,
		}))
	}

	return nil
}

// ListErrors returns paginated error logs for a project.
func (s *ErrorService) ListErrors(projectID, errorType string, page, perPage int) ([]*model.ErrorLog, int, error) {
	r := repo.NewErrorLogRepo(s.pool, s.clock)

	total, err := r.Count(projectID, errorType)
	if err != nil {
		return nil, 0, fmt.Errorf("count errors: %w", err)
	}

	offset := (page - 1) * perPage
	errors, err := r.List(projectID, errorType, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list errors: %w", err)
	}

	return errors, total, nil
}
