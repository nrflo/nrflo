package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// PhaseStatus represents the status of a phase within a workflow instance.
// Used by service.derivePhaseStatuses — not stored in workflow_instances table.
type PhaseStatus struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// WorkflowInstanceStatus represents the status of a workflow instance
type WorkflowInstanceStatus string

const (
	WorkflowInstanceActive           WorkflowInstanceStatus = "active"
	WorkflowInstanceCompleted        WorkflowInstanceStatus = "completed"
	WorkflowInstanceFailed           WorkflowInstanceStatus = "failed"
	WorkflowInstanceProjectCompleted WorkflowInstanceStatus = "project_completed"
)

// WorkflowInstance represents a running workflow on a ticket or project
type WorkflowInstance struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"project_id"`
	TicketID      string                 `json:"ticket_id"`
	WorkflowID    string                 `json:"workflow_id"`
	ScopeType     string                 `json:"scope_type"` // "ticket" or "project"
	Status        WorkflowInstanceStatus `json:"status"`
	Findings      string                 `json:"-"` // JSON object string
	SkipTags      string                 `json:"-"` // JSON array of skip tag strings
	RetryCount    int                    `json:"retry_count"`
	ParentSession sql.NullString         `json:"-"`
	WorktreePath  sql.NullString         `json:"-"`
	BranchName    sql.NullString         `json:"-"`
	EndlessLoop                   bool   `json:"endless_loop"`
	StopEndlessLoopAfterIteration bool   `json:"stop_endless_loop_after_iteration"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// IsProjectScope returns true if this is a project-scoped workflow instance
func (wi *WorkflowInstance) IsProjectScope() bool {
	return wi.ScopeType == "project"
}

// GetFindings returns the workflow-level findings as a map
func (wi *WorkflowInstance) GetFindings() map[string]interface{} {
	findings := make(map[string]interface{})
	json.Unmarshal([]byte(wi.Findings), &findings)
	return findings
}

// SetFindings updates the findings JSON string from a map
func (wi *WorkflowInstance) SetFindings(findings map[string]interface{}) {
	data, _ := json.Marshal(findings)
	wi.Findings = string(data)
}

// GetSkipTags returns the parsed skip tags as a string slice
func (wi *WorkflowInstance) GetSkipTags() []string {
	var tags []string
	if wi.SkipTags != "" {
		json.Unmarshal([]byte(wi.SkipTags), &tags)
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

	var findings map[string]interface{}
	json.Unmarshal([]byte(wi.Findings), &findings)
	if findings == nil {
		findings = make(map[string]interface{})
	}

	scopeType := wi.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	skipTags := wi.GetSkipTags()

	return json.Marshal(&struct {
		ID            string                 `json:"id"`
		ProjectID     string                 `json:"project_id"`
		TicketID      string                 `json:"ticket_id,omitempty"`
		WorkflowID    string                 `json:"workflow_id"`
		ScopeType     string                 `json:"scope_type"`
		Status        WorkflowInstanceStatus `json:"status"`
		Findings      map[string]interface{} `json:"findings"`
		SkipTags      []string               `json:"skip_tags"`
		RetryCount    int                    `json:"retry_count"`
		ParentSession *string                `json:"parent_session,omitempty"`
		WorktreePath  *string                `json:"worktree_path,omitempty"`
		BranchName    *string                `json:"branch_name,omitempty"`
		EndlessLoop                   bool   `json:"endless_loop"`
		StopEndlessLoopAfterIteration bool   `json:"stop_endless_loop_after_iteration"`
		CreatedAt     time.Time              `json:"created_at"`
		UpdatedAt     time.Time              `json:"updated_at"`
	}{
		ID:            wi.ID,
		ProjectID:     wi.ProjectID,
		TicketID:      wi.TicketID,
		WorkflowID:    wi.WorkflowID,
		ScopeType:     scopeType,
		Status:        wi.Status,
		Findings:      findings,
		SkipTags:      skipTags,
		RetryCount:    wi.RetryCount,
		ParentSession: parentSession,
		WorktreePath:  worktreePath,
		BranchName:    branchName,
		EndlessLoop:                   wi.EndlessLoop,
		StopEndlessLoopAfterIteration: wi.StopEndlessLoopAfterIteration,
		CreatedAt:     wi.CreatedAt,
		UpdatedAt:     wi.UpdatedAt,
	})
}
