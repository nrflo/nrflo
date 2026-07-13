package service

import (
	"be/internal/db"
)

// Plan manifest caps (config KV: project override > global > default).
const (
	PlanMaxLayersKey           = "plan_max_layers"
	PlanMaxNodesKey            = "plan_max_nodes"
	PlanMaxInstructionBytesKey = "plan_max_instruction_bytes"
	PlanMaxQuestionsKey        = "plan_max_questions"
	PlanDraftTTLMinKey         = "plan_draft_ttl_min"

	DefaultPlanMaxLayers           = 4
	DefaultPlanMaxNodes            = 25
	DefaultPlanMaxInstructionBytes = 4000
	DefaultPlanMaxQuestions        = 10
	DefaultPlanDraftTTLMin         = 1440
)

// PlanLimits is the resolved set of caps for a project, read once per
// validation/materialization call.
type PlanLimits struct {
	MaxLayers           int
	MaxNodes            int
	MaxInstructionBytes int
	MaxQuestions        int
	DraftTTLMin         int
}

// LoadPlanLimits reads the plan_* caps from the config KV using the same
// project>global>default cascade as SubworkflowCap.
func LoadPlanLimits(pool *db.Pool, projectID string) PlanLimits {
	return PlanLimits{
		MaxLayers:           SubworkflowCap(pool, projectID, PlanMaxLayersKey, DefaultPlanMaxLayers),
		MaxNodes:            SubworkflowCap(pool, projectID, PlanMaxNodesKey, DefaultPlanMaxNodes),
		MaxInstructionBytes: SubworkflowCap(pool, projectID, PlanMaxInstructionBytesKey, DefaultPlanMaxInstructionBytes),
		MaxQuestions:        SubworkflowCap(pool, projectID, PlanMaxQuestionsKey, DefaultPlanMaxQuestions),
		DraftTTLMin:         SubworkflowCap(pool, projectID, PlanDraftTTLMinKey, DefaultPlanDraftTTLMin),
	}
}
