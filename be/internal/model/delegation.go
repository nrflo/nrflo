package model

import "time"

// Delegation is one durable delegate-fanout tracking row (migration 000216),
// replacing the transient `_delegation_<id>` finding. Written once at seed
// time and never deleted — only marked completed/failed and consumed once
// GetDelegation reads a terminal result.
type Delegation struct {
	ID                 string
	CallerSessionID    string
	WorkflowInstanceID string
	ProjectID          string
	Tier               string
	Brief              string
	Fanout             int
	WorkerSessionIDs   []string
	SpawnErrors        []string
	Depth              int
	FanoutDone         bool
	Status             string
	CreatedAt          time.Time
	CompletedAt        *time.Time
	ConsumedAt         *time.Time

	// Worktree isolation metadata (migration 000224), set only when this
	// delegation ran under prepareDelegateWorktree. WorktreePath/BranchName
	// are persisted at fanout start; BaseCommit/Summary are filled in by
	// finalizeDelegateWorktree once the fanout's workers finish.
	WorktreePath string
	BranchName   string
	BaseCommit   string
	Summary      string
}
