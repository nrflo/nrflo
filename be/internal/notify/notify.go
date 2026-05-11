// Package notify implements the notification dispatch subsystem.
// A Dispatcher (ws.Listener) watches 5 event types, resolves enabled channels,
// and inserts pending delivery rows. A Worker drains the queue with exponential
// backoff and broadcasts WS events on success or giving_up.
package notify

import (
	"context"
	"encoding/json"

	"be/internal/logger"
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

// ProjectLookup resolves a project ID to its display name.
type ProjectLookup interface {
	Get(projectID string) (name string, ok bool, err error)
}

// TicketLookup resolves a ticket to its display title.
type TicketLookup interface {
	Get(projectID, ticketID string) (title string, ok bool, err error)
}

// ProjectLookupFunc is a function adapter implementing ProjectLookup.
type ProjectLookupFunc func(projectID string) (string, bool, error)

func (f ProjectLookupFunc) Get(projectID string) (string, bool, error) { return f(projectID) }

// TicketLookupFunc is a function adapter implementing TicketLookup.
type TicketLookupFunc func(projectID, ticketID string) (string, bool, error)

func (f TicketLookupFunc) Get(projectID, ticketID string) (string, bool, error) {
	return f(projectID, ticketID)
}

// Dispatcher implements ws.Listener and enqueues notification deliveries.
type Dispatcher struct {
	channelRepo   *repo.NotificationChannelRepo
	deliveryRepo  *repo.NotificationDeliveryRepo
	projectLookup ProjectLookup
	ticketLookup  TicketLookup
	wakeCh        chan struct{}
}

// NewDispatcher creates a Dispatcher. projectLookup and ticketLookup may be nil.
func NewDispatcher(
	channelRepo *repo.NotificationChannelRepo,
	deliveryRepo *repo.NotificationDeliveryRepo,
	projectLookup ProjectLookup,
	ticketLookup TicketLookup,
	wakeCh chan struct{},
) *Dispatcher {
	return &Dispatcher{
		channelRepo:   channelRepo,
		deliveryRepo:  deliveryRepo,
		projectLookup: projectLookup,
		ticketLookup:  ticketLookup,
		wakeCh:        wakeCh,
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

	enriched := make(map[string]interface{}, len(event.Data)+6)
	for k, v := range event.Data {
		enriched[k] = v
	}
	enriched["event_type"] = event.Type
	if event.TicketID != "" {
		enriched["ticket_id"] = event.TicketID
	}
	if event.ProjectID != "" {
		enriched["project_id"] = event.ProjectID
	}
	if event.Workflow != "" {
		enriched["workflow"] = event.Workflow
	}

	// Enrich with resolved names (best-effort; lookup errors are logged, not fatal).
	if d.projectLookup != nil {
		if name, ok, err := d.projectLookup.Get(event.ProjectID); err != nil {
			logger.Warn(context.Background(), "notify: project lookup failed", "project_id", event.ProjectID, "error", err)
		} else if ok {
			enriched["project_name"] = name
		}
	}
	if event.TicketID != "" && d.ticketLookup != nil {
		if title, ok, err := d.ticketLookup.Get(event.ProjectID, event.TicketID); err != nil {
			logger.Warn(context.Background(), "notify: ticket lookup failed", "project_id", event.ProjectID, "ticket_id", event.TicketID, "error", err)
		} else if ok {
			enriched["ticket_name"] = title
		}
	}

	payloadBytes, _ := json.Marshal(enriched)
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
