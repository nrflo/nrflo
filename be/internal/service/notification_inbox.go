package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

type NotificationInboxService struct {
	repo *repo.NotificationInboxRepo
	hub  *ws.Hub
}

func NewNotificationInboxService(pool *db.Pool, clk clock.Clock, hub *ws.Hub) *NotificationInboxService {
	return &NotificationInboxService{repo: repo.NewNotificationInboxRepo(pool, clk), hub: hub}
}

func (s *NotificationInboxService) List(project *string, limit, offset int) (*model.NotificationInbox, error) {
	return s.repo.List(project, limit, offset)
}

func (s *NotificationInboxService) MarkRead(project *string, id string) error {
	if err := s.repo.MarkRead(project, id); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.BroadcastGlobal(ws.NewEvent(ws.EventGlobalNotifications, "", "", "", nil))
	}
	return nil
}

func (s *NotificationInboxService) Get(project *string, id string) (*model.InboxNotification, error) {
	return s.repo.Get(project, id)
}
