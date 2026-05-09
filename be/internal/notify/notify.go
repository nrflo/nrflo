// Package notify implements the notification dispatch subsystem.
// A Dispatcher (ws.Listener) watches 5 event types, resolves enabled channels,
// and inserts pending delivery rows. A Worker drains the queue with exponential
// backoff and broadcasts WS events on success or giving_up.
package notify

import (
	"encoding/json"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// watchedEvents is the set of event types the Dispatcher reacts to.
var watchedEvents = map[string]bool{
	ws.EventOrchestrationCompleted: true,
	ws.EventOrchestrationFailed:    true,
	ws.EventAgentCompleted:         true,
	ws.EventAgentContextSaving:     true,
	ws.EventAgentStallRestart:      true,
}

// Dispatcher implements ws.Listener and enqueues notification deliveries.
type Dispatcher struct {
	channelRepo  *repo.NotificationChannelRepo
	deliveryRepo *repo.NotificationDeliveryRepo
	wakeCh       chan struct{}
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(
	channelRepo *repo.NotificationChannelRepo,
	deliveryRepo *repo.NotificationDeliveryRepo,
	wakeCh chan struct{},
) *Dispatcher {
	return &Dispatcher{
		channelRepo:  channelRepo,
		deliveryRepo: deliveryRepo,
		wakeCh:       wakeCh,
	}
}

// OnEvent is called by ws.Hub for every broadcast event.
func (d *Dispatcher) OnEvent(event *ws.Event) {
	if !watchedEvents[event.Type] {
		return
	}
	// For agent.completed only notify when the agent failed.
	if event.Type == ws.EventAgentCompleted {
		result, _ := event.Data["result"].(string)
		if result != "fail" {
			return
		}
	}

	projectID := event.ProjectID
	if projectID == "" {
		return
	}

	workflowID := event.Workflow
	if workflowID == "" {
		return
	}

	channels, err := d.channelRepo.ListEnabledForEvent(projectID, workflowID, event.Type)
	if err != nil || len(channels) == 0 {
		return
	}

	payloadBytes, _ := json.Marshal(event.Data)
	payloadStr := string(payloadBytes)

	for _, ch := range channels {
		// Validate kind is supported; worker will render body at dispatch time.
		if ch.Kind != model.ChannelKindSlack && ch.Kind != model.ChannelKindTelegram {
			continue
		}

		delivery := &model.NotificationDelivery{
			ChannelID: ch.ID,
			ProjectID: projectID,
			EventType: event.Type,
			Payload:   payloadStr,
			Status:    model.DeliveryStatusPending,
		}
		_ = d.deliveryRepo.Insert(delivery)
	}

	// Non-blocking wake signal.
	select {
	case d.wakeCh <- struct{}{}:
	default:
	}
}
