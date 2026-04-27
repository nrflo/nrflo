package service

import "be/internal/ws"

// WSHub is the minimum surface needed by BroadcastFromCtx.
// Both *ws.Hub and test fakes satisfy it via duck typing.
type WSHub interface {
	Broadcast(event *ws.Event)
}

// BroadcastFromCtx is the single source of truth for unpacking a BroadcastCtx
// and emitting a WebSocket event. Used by socket handlers and the future API
// tool dispatcher (T4). Nil-safe when hub is nil.
func BroadcastFromCtx(hub WSHub, eventType string, bc BroadcastCtx, data map[string]interface{}) {
	if hub == nil {
		return
	}
	hub.Broadcast(ws.NewEvent(eventType, bc.ProjectID, bc.TicketID, bc.Workflow, data))
}
