package ws

// Event types for WebSocket messages
const (
	EventAgentStarted                = "agent.started"
	EventAgentCompleted              = "agent.completed"
	EventAgentContinued              = "agent.continued"
	EventPhaseStarted                = "phase.started"
	EventPhaseCompleted              = "phase.completed"
	EventFindingsUpdated             = "findings.updated"
	EventMessagesUpdated             = "messages.updated"
	EventWorkflowUpdated             = "workflow.updated"
	EventWorkflowDefCreated          = "workflow_def.created"
	EventWorkflowDefUpdated          = "workflow_def.updated"
	EventWorkflowDefDeleted          = "workflow_def.deleted"
	EventAgentDefCreated             = "agent_def.created"
	EventAgentDefUpdated             = "agent_def.updated"
	EventAgentDefDeleted             = "agent_def.deleted"
	EventSystemAgentDefCreated       = "system_agent_def.created"
	EventSystemAgentDefUpdated       = "system_agent_def.updated"
	EventTierModelsUpdated           = "tier_models.updated"
	EventSystemAgentDefDeleted       = "system_agent_def.deleted"
	EventTicketUpdated               = "ticket.updated"
	EventOrchestrationStarted        = "orchestration.started"
	EventOrchestrationCompleted      = "orchestration.completed"
	EventOrchestrationFailed         = "orchestration.failed"
	EventOrchestrationRetried        = "orchestration.retried"
	EventOrchestrationCallback       = "orchestration.callback"
	EventChainUpdated                = "chain.updated"
	EventProjectFindingsUpdated      = "project_findings.updated"
	EventAgentContextUpdated         = "agent.context_updated"
	EventSessionCostUpdated          = "session.cost_updated"
	EventAgentContextLedger          = "agent.context_ledger"
	EventAgentHandoffDigest          = "agent.handoff_digest"
	EventAgentTakeControl            = "agent.take_control"
	EventAgentKilled                 = "agent.killed"
	EventAgentTakeControlRejected    = "agent.take_control_rejected"
	EventAgentViewerAttached         = "agent.viewer_attached"
	EventLayerSkipped                = "layer.skipped"
	EventAgentRetryWaiting           = "agent.retry_waiting"
	EventAgentStallWaiting           = "agent.stall_waiting"
	EventAgentStallRestart           = "agent.stall_restart"
	EventAgentRateLimited            = "agent.rate_limited"
	EventAgentRateLimitsUpdated      = "agent.rate_limits_updated"
	EventAgentNudged                 = "agent.nudged"
	EventAgentContextSaving          = "agent.context_saving"
	EventAgentProviderFallback       = "agent.provider_fallback"
	EventSkipTagAdded                = "skip_tag.added"
	EventMergeConflictResolving      = "merge.conflict_resolving"
	EventMergeConflictResolved       = "merge.conflict_resolved"
	EventMergeConflictFailed         = "merge.conflict_failed"
	EventWorkflowInstanceDeleted     = "workflow_instance.deleted"
	EventWorkflowPurged              = "workflow.purged"
	EventDefaultTemplateCreated      = "default_template.created"
	EventDefaultTemplateUpdated      = "default_template.updated"
	EventDefaultTemplateDeleted      = "default_template.deleted"
	EventModelCreated                = "model.created"
	EventModelUpdated                = "model.updated"
	EventModelDeleted                = "model.deleted"
	EventCustomProviderCreated       = "custom_provider.created"
	EventCustomProviderUpdated       = "custom_provider.updated"
	EventCustomProviderDeleted       = "custom_provider.deleted"
	EventErrorCreated                = "error.created"
	EventScheduleCreated             = "schedule.created"
	EventScheduleDeleted             = "schedule.deleted"
	EventScheduleTriggered           = "schedule.triggered"
	EventScheduleUpdated             = "schedule.updated"
	EventWorkflowPushed              = "workflow.pushed"
	EventWorkflowPushFailed          = "workflow.push_failed"
	EventWorkflowFinalizeSucceeded   = "workflow.finalize_succeeded"
	EventWorkflowFinalizeFailed      = "workflow.finalize_failed"
	EventWorkflowPaused              = "workflow.paused"
	EventWorkflowResumed             = "workflow.resumed"
	EventTestEcho                    = "test.echo"
	EventNotificationChannelCreated  = "notification_channel.created"
	EventNotificationChannelUpdated  = "notification_channel.updated"
	EventNotificationChannelDeleted  = "notification_channel.deleted"
	EventNotificationDelivered       = "notification.delivered"
	EventNotificationFailed          = "notification.failed"
	EventToolDispatched              = "tool.dispatched"
	EventWorkflowChainCreated        = "chain_def.created"
	EventWorkflowChainUpdated        = "chain_def.updated"
	EventWorkflowChainDeleted        = "chain_def.deleted"
	EventChainRunStarted             = "chain.run_started"
	EventChainStepStarted            = "chain.step_started"
	EventChainStepCompleted          = "chain.step_completed"
	EventChainRunCompleted           = "chain.run_completed"
	EventChainRunFailed              = "chain.run_failed"
	EventArtifactCreated             = "artifact.created"
	EventArtifactDeleted             = "artifact.deleted"
	EventConsultStarted              = "consult.started"
	EventConsultAnswered             = "consult.answered"
	EventConsultFailed               = "consult.failed"
	EventDelegateStarted             = "delegate.started"
	EventDelegateCompleted           = "delegate.completed"
	EventDelegateFailed              = "delegate.failed"
	EventConsoleChatDelta            = "console_chat.delta"
	EventConsoleChatTurn             = "console_chat.turn"
	EventConsoleChatApprovalRequest  = "console_chat.approval_request"
	EventConsoleChatApprovalResolved = "console_chat.approval_resolved"
	EventConsoleChatError            = "console_chat.error"
	EventConsoleChatThinking         = "console_chat.thinking"
	EventConsoleChatSessionApprovals = "console_chat.session_approvals"
	EventConsoleChatSiblingOpened    = "console_chat.sibling_opened"
	EventConsoleContextRotated       = "console.context_rotated"
	EventRefineryFoldFailed          = "refinery.fold_failed"
	EventStepAdvanced                = "step.advanced"
)

// Event represents a WebSocket event to broadcast
type Event struct {
	ProtocolVersion int                    `json:"protocol_version,omitempty"`
	Type            string                 `json:"type"`
	ProjectID       string                 `json:"project_id"`
	TicketID        string                 `json:"ticket_id"`
	Workflow        string                 `json:"workflow,omitempty"`
	Timestamp       string                 `json:"timestamp"`
	Sequence        int64                  `json:"sequence,omitempty"`
	Entity          string                 `json:"entity,omitempty"`
	Data            map[string]interface{} `json:"data,omitempty"`
	SessionID       string                 `json:"session_id,omitempty"`
}

// NewEvent creates a new event. Timestamp is assigned later by Hub.broadcastEvent().
func NewEvent(eventType, projectID, ticketID, workflow string, data map[string]interface{}) *Event {
	return &Event{
		Type:      eventType,
		ProjectID: projectID,
		TicketID:  ticketID,
		Workflow:  workflow,
		Data:      data,
	}
}
