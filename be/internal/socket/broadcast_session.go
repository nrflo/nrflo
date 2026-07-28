package socket

import (
	"be/internal/service"
	"be/internal/ws"
)

// broadcastMessageEvent emits eventType on the project-scoped channel when
// projectID resolves (service.BroadcastFromCtx — unchanged gating, since the
// AgentService broadcast-id lookup INNER JOINs workflow_instances and returns
// empty ids for a console-chat session) and ALWAYS also on sessionID's WS
// session channel. This is what makes hook-recorded rows for a chat session —
// whose project broadcast is structurally skipped — reach a live subscriber.
// No kind check anywhere (Rule 6): a workflow-agent session simply has no
// session-channel subscribers, so the extra push is a no-op there.
func broadcastMessageEvent(hub *ws.Hub, eventType, projectID, ticketID, workflow, sessionID string, data map[string]interface{}) {
	if projectID != "" {
		service.BroadcastFromCtx(hub, eventType, service.BroadcastCtx{
			SessionID: sessionID,
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflow,
		}, data)
	}
	if hub == nil {
		return
	}
	hub.BroadcastSession(&ws.Event{
		Type:      eventType,
		SessionID: sessionID,
		ProjectID: projectID,
		Data:      data,
	})
}
