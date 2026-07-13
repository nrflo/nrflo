package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// PhaseStatus represents the status of a phase within a workflow instance.
// Used by service.derivePhaseStatuses — not stored in workflow_instances table.
// status values: pending, in_progress, completed, skipped, rate_limited.
type PhaseStatus struct {
	Status              string `json:"status"`
	Result              string `json:"result,omitempty"`
	RateLimitUntilTs    string `json:"rate_limit_until_ts,omitempty"`
	RateLimitRetryCount int    `json:"rate_limit_retry_count,omitempty"`
}

// WorkflowInstanceStatus represents the status of a workflow instance
type WorkflowInstanceStatus string

const (
	WorkflowInstanceActive           WorkflowInstanceStatus = "active"
	WorkflowInstanceCompleted        WorkflowInstanceStatus = "completed"
	WorkflowInstanceFailed           WorkflowInstanceStatus = "failed"
	WorkflowInstanceProjectCompleted WorkflowInstanceStatus = "project_completed"
	WorkflowInstanceWaiting          WorkflowInstanceStatus = "waiting"

	// Plan-boundary suspend statuses (DYNWF-5): the engine has exhausted its
	// static layers on a plan-driven workflow and is parked waiting on the
	// plan lifecycle (draft/revise/approve), not the pause_after machinery.
	WorkflowInstancePlanning        WorkflowInstanceStatus = "planning"
	WorkflowInstancePlanReady       WorkflowInstanceStatus = "plan_ready"
	WorkflowInstanceWaitingInput    WorkflowInstanceStatus = "waiting_input"
	WorkflowInstanceWaitingApproval WorkflowInstanceStatus = "waiting_approval"
)

// IsPlanSuspended returns true for the four plan-boundary statuses — the
// engine is parked at the plan boundary, not running and not pause_after-waiting.
func IsPlanSuspended(status WorkflowInstanceStatus) bool {
	switch status {
	case WorkflowInstancePlanning, WorkflowInstancePlanReady, WorkflowInstanceWaitingInput, WorkflowInstanceWaitingApproval:
		return true
	default:
		return false
	}
}

// WorkflowInstance represents a running workflow on a ticket or project
type WorkflowInstance struct {
	ID                            string                 `json:"id"`
	ProjectID                     string                 `json:"project_id"`
	TicketID                      string                 `json:"ticket_id"`
	WorkflowID                    string                 `json:"workflow_id"`
	ScopeType                     string                 `json:"scope_type"` // "ticket" or "project"
	Status                        WorkflowInstanceStatus `json:"status"`
	SkipTags                      string                 `json:"-"` // JSON array of skip tag strings
	RetryCount                    int                    `json:"retry_count"`
	ParentSession                 sql.NullString         `json:"-"`
	WorktreePath                  sql.NullString         `json:"-"`
	BranchName                    sql.NullString         `json:"-"`
	EndlessLoop                   bool                   `json:"endless_loop"`
	StopEndlessLoopAfterIteration bool                   `json:"stop_endless_loop_after_iteration"`
	PlanAutoApprove               bool                   `json:"plan_auto_approve"`            // mode=auto for the self-drafting plan boundary (DYNWF-6), gated by service.DynamicAutoEnabled
	PurgeOnCompletion             bool                   `json:"purge_on_completion"`          // snapshot of the workflow def flag at creation; drives terminal-state trace purge
	LaunchDepth                   int                    `json:"launch_depth"`                 // lineage distance from a human/scheduler start; incremented by run_subworkflow and next_workflow_on_success (chain cap)
	ParentInstanceID              string                 `json:"parent_instance_id,omitempty"` // run_subworkflow caller's instance id; "" for top-level and chain runs
	SubworkflowDepth              int                    `json:"subworkflow_depth"`            // nesting via run_subworkflow only (chain hops carry it unchanged)
	SubworkflowStarts             int                    `json:"-"`                            // persisted run_subworkflow invocation budget charged to this run
	ScheduledTaskID               string                 `json:"scheduled_task_id,omitempty"`
	ExternalID                    string                 `json:"external_id,omitempty"`
	ExternalContext               string                 `json:"external_context,omitempty"`
	CreatedAt                     time.Time              `json:"created_at"`
	UpdatedAt                     time.Time              `json:"updated_at"`
}

// IsProjectScope returns true if this is a project-scoped workflow instance
func (wi *WorkflowInstance) IsProjectScope() bool {
	return wi.ScopeType == "project"
}

// GetSkipTags returns the parsed skip tags as a string slice
func (wi *WorkflowInstance) GetSkipTags() []string {
	var tags []string
	if wi.SkipTags != "" {
		_ = json.Unmarshal([]byte(wi.SkipTags), &tags)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags
}

// SetSkipTags sets the skip tags from a string slice
func (wi *WorkflowInstance) SetSkipTags(tags []string) {
	if tags == nil {
		tags = []string{}
	}
	data, _ := json.Marshal(tags)
	wi.SkipTags = string(data)
}

// AddSkipTag appends a skip tag without duplicates
func (wi *WorkflowInstance) AddSkipTag(tag string) {
	tags := wi.GetSkipTags()
	for _, t := range tags {
		if t == tag {
			return
		}
	}
	tags = append(tags, tag)
	wi.SetSkipTags(tags)
}

// MarshalJSON implements custom JSON marshaling for WorkflowInstance
func (wi WorkflowInstance) MarshalJSON() ([]byte, error) {
	var parentSession *string
	if wi.ParentSession.Valid {
		parentSession = &wi.ParentSession.String
	}
	var worktreePath *string
	if wi.WorktreePath.Valid {
		worktreePath = &wi.WorktreePath.String
	}
	var branchName *string
	if wi.BranchName.Valid {
		branchName = &wi.BranchName.String
	}

	scopeType := wi.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	skipTags := wi.GetSkipTags()

	return json.Marshal(&struct {
		ID                            string                 `json:"id"`
		ProjectID                     string                 `json:"project_id"`
		TicketID                      string                 `json:"ticket_id,omitempty"`
		WorkflowID                    string                 `json:"workflow_id"`
		ScopeType                     string                 `json:"scope_type"`
		Status                        WorkflowInstanceStatus `json:"status"`
		SkipTags                      []string               `json:"skip_tags"`
		RetryCount                    int                    `json:"retry_count"`
		ParentSession                 *string                `json:"parent_session,omitempty"`
		WorktreePath                  *string                `json:"worktree_path,omitempty"`
		BranchName                    *string                `json:"branch_name,omitempty"`
		EndlessLoop                   bool                   `json:"endless_loop"`
		StopEndlessLoopAfterIteration bool                   `json:"stop_endless_loop_after_iteration"`
		PlanAutoApprove               bool                   `json:"plan_auto_approve"`
		PurgeOnCompletion             bool                   `json:"purge_on_completion"`
		LaunchDepth                   int                    `json:"launch_depth"`
		ParentInstanceID              string                 `json:"parent_instance_id,omitempty"`
		SubworkflowDepth              int                    `json:"subworkflow_depth"`
		ScheduledTaskID               string                 `json:"scheduled_task_id,omitempty"`
		ExternalID                    string                 `json:"external_id,omitempty"`
		ExternalContext               string                 `json:"external_context,omitempty"`
		CreatedAt                     time.Time              `json:"created_at"`
		UpdatedAt                     time.Time              `json:"updated_at"`
	}{
		ID:                            wi.ID,
		ProjectID:                     wi.ProjectID,
		TicketID:                      wi.TicketID,
		WorkflowID:                    wi.WorkflowID,
		ScopeType:                     scopeType,
		Status:                        wi.Status,
		SkipTags:                      skipTags,
		RetryCount:                    wi.RetryCount,
		ParentSession:                 parentSession,
		WorktreePath:                  worktreePath,
		BranchName:                    branchName,
		EndlessLoop:                   wi.EndlessLoop,
		StopEndlessLoopAfterIteration: wi.StopEndlessLoopAfterIteration,
		PlanAutoApprove:               wi.PlanAutoApprove,
		PurgeOnCompletion:             wi.PurgeOnCompletion,
		LaunchDepth:                   wi.LaunchDepth,
		ParentInstanceID:              wi.ParentInstanceID,
		SubworkflowDepth:              wi.SubworkflowDepth,
		ScheduledTaskID:               wi.ScheduledTaskID,
		ExternalID:                    wi.ExternalID,
		ExternalContext:               wi.ExternalContext,
		CreatedAt:                     wi.CreatedAt,
		UpdatedAt:                     wi.UpdatedAt,
	})
}
