package service

import "be/internal/ws"

// WSHub is the minimum surface needed by BroadcastFromCtx.
// Both *ws.Hub and test fakes satisfy it via duck typing.
type WSHub interface {
	Broadcast(event *ws.Event)
}

// BroadcastFromCtx is the single source of truth for unpacking a BroadcastCtx
// and emitting a WebSocket event. Used by socket handlers and the future API
// tool dispatcher (T4). Nil-safe when hub is nil. Stamps Event.SessionID from
// bc.SessionID so out-of-band ws.Listeners (e.g. the spawner's proactive-
// restart task-boundary tap) can attribute a session-scoped broadcast like
// findings.updated back to the session that produced it.
func BroadcastFromCtx(hub WSHub, eventType string, bc BroadcastCtx, data map[string]interface{}) {
	if hub == nil {
		return
	}
	ev := ws.NewEvent(eventType, bc.ProjectID, bc.TicketID, bc.Workflow, data)
	ev.SessionID = bc.SessionID
	hub.Broadcast(ev)
}
