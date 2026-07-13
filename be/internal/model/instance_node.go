package model

import "time"

// InstanceNode is a single materialized plan node, immutable once written by
// service.PlanService.Materialize. NodeID is execution identity (layer group
// membership, retry target); AgentType is the fanout_template def it was
// bound to at materialization time (model/tag/prompt resolution).
type InstanceNode struct {
	InstanceID   string    `json:"instance_id"`
	NodeID       string    `json:"node_id"`
	Layer        int       `json:"layer"`
	AgentType    string    `json:"agent_type"`
	Instructions string    `json:"instructions"`
	PlanRevision int       `json:"plan_revision"`
	CreatedAt    time.Time `json:"created_at"`
}

// InstanceLayerPolicy is a materialized plan layer's fan-in pass policy,
// merged over the def-scoped workflow_layer_policies map at read/run time.
type InstanceLayerPolicy struct {
	InstanceID string `json:"instance_id"`
	Layer      int    `json:"layer"`
	PassPolicy string `json:"pass_policy"`
}
