package ws

import (
	"encoding/json"
	"time"
)

// streamSnapshot sends snapshot.begin, snapshot.chunk(s), snapshot.end to a client.
// After snapshot, the client continues with live events from the current seq onward.
func streamSnapshot(c *Client, projectID, ticketID string, hub *Hub) {
	sp := hub.snapshotProvider
	if sp == nil {
		sendControlEvent(c, EventResyncRequired, projectID, ticketID, nil)
		return
	}

	chunks, err := sp.BuildSnapshot(projectID, ticketID)
	if err != nil {
		sendControlEvent(c, EventResyncRequired, projectID, ticketID, nil)
		return
	}

	// Capture current seq before sending snapshot
	var currentSeq int64
	if el := hub.GetEventLog(); el != nil {
		currentSeq, _ = el.LatestSeq(projectID, ticketID)
	}

	ts := hub.clock.Now().UTC().Format(time.RFC3339Nano)

	// snapshot.begin
	sendControlEvent(c, EventSnapshotBegin, projectID, ticketID, map[string]interface{}{
		"chunk_count": len(chunks),
	})

	// snapshot.chunk per entity
	for _, chunk := range chunks {
		evt := &Event{
			ProtocolVersion: ProtocolVersion,
			Type:            EventSnapshotChunk,
			ProjectID:       projectID,
			TicketID:        ticketID,
			Timestamp:       ts,
			Entity:          chunk.Entity,
			Data:            chunk.Data,
		}
		evtData, _ := json.Marshal(evt)
		select {
		case c.send <- evtData:
		default:
			return
		}
	}

	// snapshot.end with current seq so client knows where live events resume
	sendControlEvent(c, EventSnapshotEnd, projectID, ticketID, map[string]interface{}{
		"current_seq": currentSeq,
	})
}
