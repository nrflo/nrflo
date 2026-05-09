package service

// TODO: encrypt at rest

import (
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
	"be/internal/ws"
)

// NotificationWaker signals the delivery worker to wake up.
type NotificationWaker interface {
	Wake()
}

// chanWaker wraps a chan struct{} as a NotificationWaker.
type chanWaker struct{ ch chan struct{} }

func (w *chanWaker) Wake() {
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// NewChanWaker wraps wakeCh as a NotificationWaker.
func NewChanWaker(wakeCh chan struct{}) NotificationWaker { return &chanWaker{ch: wakeCh} }

// NotificationService handles notification channel CRUD.
type NotificationService struct {
	pool  *db.Pool
	clk   clock.Clock
	hub   *ws.Hub
	waker NotificationWaker
	wfSvc *WorkflowService
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(pool *db.Pool, clk clock.Clock, hub *ws.Hub, waker NotificationWaker, wfSvc *WorkflowService) *NotificationService {
	return &NotificationService{pool: pool, clk: clk, hub: hub, waker: waker, wfSvc: wfSvc}
}

// List returns all notification channels for a project+workflow (configs masked).
func (s *NotificationService) List(projectID, workflowID string) ([]*model.NotificationChannel, error) {
	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	channels, err := r.ListByWorkflow(projectID, workflowID)
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []*model.NotificationChannel{}
	}
	for _, ch := range channels {
		ch.Config = maskConfig(string(ch.Kind), ch.Config)
	}
	return channels, nil
}

// Get returns a single channel by ID (config masked).
func (s *NotificationService) Get(id string) (*model.NotificationChannel, error) {
	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	ch, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	ch.Config = maskConfig(string(ch.Kind), ch.Config)
	return ch, nil
}

// Create validates and persists a new notification channel for a specific workflow.
func (s *NotificationService) Create(projectID, workflowID string, req *types.NotificationChannelCreateRequest) (*model.NotificationChannel, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Kind != "slack" && req.Kind != "telegram" {
		return nil, fmt.Errorf("kind must be slack or telegram")
	}

	if s.wfSvc != nil {
		if _, err := s.wfSvc.GetWorkflowDef(projectID, workflowID); err != nil {
			return nil, fmt.Errorf("workflow not found: %s", workflowID)
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configJSON := "{}"
	if req.Config != nil {
		b, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid config: %w", err)
		}
		configJSON = string(b)
	}

	eventTypes := req.EventTypes
	if eventTypes == nil {
		eventTypes = []string{}
	}

	ch := &model.NotificationChannel{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Name:       req.Name,
		Kind:       model.ChannelKind(req.Kind),
		Enabled:    enabled,
		Config:     configJSON,
		EventTypes: eventTypes,
	}

	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	if err := r.Insert(ch); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("notification channel already exists: %s", ch.ID)
		}
		return nil, err
	}

	s.broadcast(ws.EventNotificationChannelCreated, projectID, workflowID, ch.ID)

	result := *ch
	result.Config = maskConfig(string(ch.Kind), ch.Config)
	return &result, nil
}

// Update applies partial PATCH updates to a notification channel.
func (s *NotificationService) Update(id string, req *types.NotificationChannelUpdateRequest) (*model.NotificationChannel, error) {
	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	ch, err := r.Get(id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		ch.Name = *req.Name
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	if req.Config != nil {
		incoming, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid config: %w", err)
		}
		ch.Config = applyConfigPatch(string(ch.Kind), ch.Config, string(incoming))
	}
	if req.EventTypes != nil {
		ch.EventTypes = *req.EventTypes
		if ch.EventTypes == nil {
			ch.EventTypes = []string{}
		}
	}

	if err := r.Update(ch); err != nil {
		return nil, err
	}

	s.broadcast(ws.EventNotificationChannelUpdated, ch.ProjectID, ch.WorkflowID, ch.ID)

	result := *ch
	result.Config = maskConfig(string(ch.Kind), ch.Config)
	return &result, nil
}

// Delete removes a notification channel.
func (s *NotificationService) Delete(id string) (string, error) {
	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	ch, err := r.Get(id)
	if err != nil {
		return "", err
	}
	projectID := ch.ProjectID
	workflowID := ch.WorkflowID
	if err := r.Delete(id); err != nil {
		return "", err
	}
	s.broadcast(ws.EventNotificationChannelDeleted, projectID, workflowID, id)
	return projectID, nil
}

// TestSend enqueues a synthetic test delivery for the channel and wakes the worker.
func (s *NotificationService) TestSend(id string) error {
	r := repo.NewNotificationChannelRepo(s.pool, s.clk)
	ch, err := r.Get(id)
	if err != nil {
		return err
	}

	delivery := &model.NotificationDelivery{
		ChannelID: ch.ID,
		ProjectID: ch.ProjectID,
		EventType: "test",
		Payload:   fmt.Sprintf(`{"message":"test notification from nrflo","workflow_id":%q}`, ch.WorkflowID),
		Status:    model.DeliveryStatusPending,
	}

	dr := repo.NewNotificationDeliveryRepo(s.pool, s.clk)
	if err := dr.Insert(delivery); err != nil {
		return err
	}

	if s.waker != nil {
		s.waker.Wake()
	}
	return nil
}

// ListDeliveries returns recent deliveries for a channel.
func (s *NotificationService) ListDeliveries(channelID string, limit int) ([]*model.NotificationDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	r := repo.NewNotificationDeliveryRepo(s.pool, s.clk)
	deliveries, err := r.ListByChannel(channelID, limit)
	if deliveries == nil {
		deliveries = []*model.NotificationDelivery{}
	}
	return deliveries, err
}

func (s *NotificationService) broadcast(eventType, projectID, workflowID, channelID string) {
	if s.hub != nil {
		s.hub.Broadcast(ws.NewEvent(eventType, projectID, "", workflowID, map[string]interface{}{
			"channel_id": channelID,
		}))
	}
}
