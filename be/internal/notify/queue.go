package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// ErrorRecorder persists actionable errors (implemented by service.ErrorService).
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}

// backoffSchedule maps attempt index (0-based) to delay before next attempt.
var backoffSchedule = []time.Duration{15 * time.Second, 60 * time.Second, 300 * time.Second}

// Worker drains the notification delivery queue.
type Worker struct {
	deliveryRepo *repo.NotificationDeliveryRepo
	channelRepo  *repo.NotificationChannelRepo
	hub          *ws.Hub
	errorSvc     ErrorRecorder
	clk          clock.Clock
	wakeCh       chan struct{}
}

// NewWorker creates a Worker.
func NewWorker(
	deliveryRepo *repo.NotificationDeliveryRepo,
	channelRepo *repo.NotificationChannelRepo,
	hub *ws.Hub,
	errorSvc ErrorRecorder,
	clk clock.Clock,
	wakeCh chan struct{},
) *Worker {
	return &Worker{
		deliveryRepo: deliveryRepo,
		channelRepo:  channelRepo,
		hub:          hub,
		errorSvc:     errorSvc,
		clk:          clk,
		wakeCh:       wakeCh,
	}
}

// Run processes pending deliveries until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		case <-w.wakeCh:
			w.tick(ctx)
		}
	}
}

// Tick processes one batch of pending deliveries. Exported for unit testing.
func (w *Worker) Tick(ctx context.Context) {
	w.tick(ctx)
}

func (w *Worker) tick(ctx context.Context) {
	deliveries, err := w.deliveryRepo.ListPending(w.clk.Now(), 20)
	if err != nil || len(deliveries) == 0 {
		return
	}
	for _, d := range deliveries {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.dispatch(d)
	}
}

func (w *Worker) dispatch(d *model.NotificationDelivery) {
	ch, err := w.channelRepo.Get(d.ChannelID)
	if err != nil {
		// Channel deleted; mark giving_up silently.
		_ = w.deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusGivingUp, d.Attempts, err.Error(), nil)
		return
	}

	var configMap map[string]interface{}
	_ = json.Unmarshal([]byte(ch.Config), &configMap)

	var eventData map[string]interface{}
	_ = json.Unmarshal([]byte(d.Payload), &eventData)

	body := Render(ch.Kind, ch.MessageTemplate, eventData)

	transport := Get(string(ch.Kind))
	if transport == nil {
		_ = w.deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusGivingUp, d.Attempts, fmt.Sprintf("no transport for kind: %s", ch.Kind), nil)
		return
	}

	sendErr := transport.Send(&Notification{
		ChannelID: ch.ID,
		Kind:      string(ch.Kind),
		Config:    configMap,
		Body:      body,
	})

	newAttempts := d.Attempts + 1

	if sendErr == nil {
		_ = w.deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusSent, newAttempts, "", nil)
		if w.hub != nil {
			w.hub.Broadcast(ws.NewEvent(ws.EventNotificationDelivered, d.ProjectID, "", "", map[string]interface{}{
				"delivery_id": d.ID,
				"channel_id":  d.ChannelID,
				"event_type":  d.EventType,
			}))
		}
		return
	}

	errMsg := sendErr.Error()

	if newAttempts >= 3 {
		_ = w.deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusGivingUp, newAttempts, errMsg, nil)
		_ = w.errorSvc.RecordError(d.ProjectID, "notification", d.ChannelID, fmt.Sprintf("notification giving up after %d attempts: %s", newAttempts, errMsg))
		if w.hub != nil {
			w.hub.Broadcast(ws.NewEvent(ws.EventNotificationFailed, d.ProjectID, "", "", map[string]interface{}{
				"delivery_id": d.ID,
				"channel_id":  d.ChannelID,
				"event_type":  d.EventType,
				"error":       errMsg,
			}))
		}
		return
	}

	// Reschedule with backoff based on current attempt count (0-indexed).
	idx := newAttempts - 1
	if idx >= len(backoffSchedule) {
		idx = len(backoffSchedule) - 1
	}
	nextAt := w.clk.Now().Add(backoffSchedule[idx])
	_ = w.deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusPending, newAttempts, errMsg, &nextAt)
}
