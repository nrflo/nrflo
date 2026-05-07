package ws

import (
	"encoding/json"
	"strings"
)

const replayBatchLimit = 1000

// handleReplay queries the event log for events after sinceSeq and streams them to the client.
// If no events are found (cursor too old or empty log), it triggers a snapshot if a provider is
// configured, otherwise sends resync.required.
func handleReplay(c *Client, projectID, ticketID string, sinceSeq int64, hub *Hub) {
	el := hub.GetEventLog()
	if el == nil {
		// No event log configured — nothing to replay
		return
	}

	projectID = strings.ToLower(projectID)
	ticketID = strings.ToLower(ticketID)

	entries, err := el.QuerySince(projectID, ticketID, sinceSeq, replayBatchLimit)
	if err != nil {
		sendControlEvent(c, EventResyncRequired, projectID, ticketID, nil)
		return
	}

	if len(entries) == 0 {
		// Cursor could be current (nothing new), too old (pruned), or
		// in the future (server reset / event log truncated while client
		// holds a higher seq from a prior server instance).
		latestSeq, _ := el.LatestSeq(projectID, ticketID)
		if sinceSeq > 0 && sinceSeq < latestSeq {
			// Events were pruned — need snapshot or resync
			if hub.snapshotProvider != nil {
				streamSnapshot(c, projectID, ticketID, hub)
				return
			}
			sendControlEvent(c, EventResyncRequired, projectID, ticketID, nil)
			return
		}
		if sinceSeq > 0 && sinceSeq > latestSeq {
			// Client cursor is ahead of the server — the server's event log
			// was reset (or the client persisted a seq from a different DB).
			// Without this branch the client would silently drop every future
			// event because its local seq > seq of newly broadcast events.
			if hub.snapshotProvider != nil {
				streamSnapshot(c, projectID, ticketID, hub)
				return
			}
			sendControlEvent(c, EventResyncRequired, projectID, ticketID, nil)
			return
		}
		if sinceSeq == 0 && hub.snapshotProvider != nil {
			// Fresh subscribe with cursor 0 — send snapshot for initial hydration
			streamSnapshot(c, projectID, ticketID, hub)
			return
		}
		// sinceSeq == latestSeq — client is caught up
		return
	}

	// Stream replay events to client
	for _, entry := range entries {
		var data map[string]interface{}
		if err := json.Unmarshal(entry.Payload, &data); err != nil {
			data = map[string]interface{}{"raw": string(entry.Payload)}
		}
		evt := &Event{
			ProtocolVersion: ProtocolVersion,
			Type:            entry.EventType,
			ProjectID:       entry.ProjectID,
			TicketID:        entry.TicketID,
			Workflow:        entry.Workflow,
			Timestamp:       entry.CreatedAt,
			Sequence:        entry.Seq,
			Data:            data,
		}
		evtData, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		select {
		case c.send <- evtData:
		default:
			return
		}
	}

}

// sendControlEvent sends a protocol v2 control event to a single client.
func sendControlEvent(c *Client, eventType, projectID, ticketID string, data map[string]interface{}) {
	evt := &Event{
		ProtocolVersion: ProtocolVersion,
		Type:            eventType,
		ProjectID:       projectID,
		TicketID:        ticketID,
		Data:            data,
	}
	evtData, _ := json.Marshal(evt)
	select {
	case c.send <- evtData:
	default:
	}
}
