package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// MaterializedPlan is the result of a successful (or idempotent-repeat)
// Materialize call.
type MaterializedPlan struct {
	Nodes         []model.InstanceNode
	LayerPolicies map[int]string
}

// Materialize turns an approved plan revision into runnable
// workflow_instance_nodes + workflow_instance_layer_policies rows, offsetting
// manifest layer numbers above the workflow definition's static executable
// layers. Transactional and idempotent via a conditional-UPDATE hash stamp on
// the plan head (materialized_revision/materialized_hash) — safe to call
// again after a crash between approve and materialize, or every time the
// orchestrator's plan boundary re-checks an already-materialized run.
func (s *PlanService) Materialize(instanceID string) (*MaterializedPlan, error) {
	head, err := s.planRepo.GetHead(instanceID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no plan for workflow instance %s", instanceID)
	}
	if err != nil {
		return nil, err
	}
	if head.Status != model.PlanStatusApproved || head.ApprovedRevision == 0 {
		return nil, fmt.Errorf("plan: instance %s has no approved revision to materialize", instanceID)
	}

	rev, err := s.planRepo.GetRevision(instanceID, head.ApprovedRevision)
	if err != nil {
		return nil, err
	}
	wfi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(instanceID)
	if err != nil {
		return nil, err
	}
	m, err := ParsePlanManifest(json.RawMessage(rev.Manifest))
	if err != nil {
		return nil, err
	}
	if err := ValidatePlanManifest(s.pool, wfi.ProjectID, wfi.WorkflowID, m); err != nil {
		return nil, fmt.Errorf("plan: manifest no longer valid at materialization: %w", err)
	}

	layerOffset, err := maxStaticExecutableLayer(s.pool, s.clock, wfi.ProjectID, wfi.WorkflowID)
	if err != nil {
		return nil, err
	}
	layerOffset++ // manifest layer L -> engine layer L+offset

	hash := HashManifest(m)
	nodeRepo := repo.NewInstanceNodeRepo(s.pool, s.clock)

	var result *MaterializedPlan
	err = db.WithBusyRetry(func() error {
		res, txErr := s.materializeOnce(instanceID, head.ApprovedRevision, hash, m, layerOffset, nodeRepo)
		if txErr != nil {
			return txErr
		}
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PlanService) materializeOnce(instanceID string, approvedRevision int, hash string, m PlanManifest, layerOffset int, nodeRepo *repo.InstanceNodeRepo) (*MaterializedPlan, error) {
	tx, err := s.pool.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.Exec(
		`UPDATE workflow_plans SET materialized_revision = ?, materialized_hash = ?
		 WHERE instance_id = ? AND status = 'approved' AND approved_revision = ? AND materialized_revision = 0`,
		approvedRevision, hash, instanceID, approvedRevision,
	)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}

	if affected == 0 {
		// Already materialized: idempotent no-op if the hash matches the
		// stored one, else the approved revision changed underneath us.
		var storedRevision int
		var storedHash string
		if err := tx.QueryRow(
			`SELECT materialized_revision, materialized_hash FROM workflow_plans WHERE instance_id = ?`, instanceID,
		).Scan(&storedRevision, &storedHash); err != nil {
			return nil, err
		}
		if storedRevision != approvedRevision || storedHash != hash {
			return nil, fmt.Errorf("plan: instance %s already materialized a different revision (stored=%d, requested=%d)", instanceID, storedRevision, approvedRevision)
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		nodes, err := nodeRepo.List(instanceID)
		if err != nil {
			return nil, err
		}
		policies, err := nodeRepo.ListLayerPolicies(instanceID)
		if err != nil {
			return nil, err
		}
		return &MaterializedPlan{Nodes: nodes, LayerPolicies: policies}, nil
	}

	var nodes []model.InstanceNode
	policies := make(map[int]string)
	for _, layer := range m.Layers {
		engineLayer := layer.Layer + layerOffset
		policy := layer.Policy
		if policy == "" {
			policy = "any"
		}
		if err := ValidateLayerPolicy(policy, len(layer.Nodes)); err != nil {
			return nil, fmt.Errorf("plan: layer %d: %w", layer.Layer, err)
		}
		policies[engineLayer] = policy
		for _, node := range layer.Nodes {
			nodes = append(nodes, model.InstanceNode{
				InstanceID:   instanceID,
				NodeID:       node.ID,
				Layer:        engineLayer,
				AgentType:    node.Template,
				Instructions: node.Instructions,
				PlanRevision: approvedRevision,
			})
		}
	}

	if err := nodeRepo.InsertNodes(tx, instanceID, nodes); err != nil {
		return nil, err
	}
	if err := nodeRepo.InsertLayerPolicies(tx, instanceID, policies); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &MaterializedPlan{Nodes: nodes, LayerPolicies: policies}, nil
}

// maxStaticExecutableLayer returns the highest layer number among the
// workflow's static executable phases (-1 if there are none), so plan layers
// can be offset above them.
func maxStaticExecutableLayer(pool *db.Pool, clk clock.Clock, projectID, workflowID string) (int, error) {
	defs, err := repo.NewAgentDefinitionRepo(pool, clk).ListExecutable(projectID, workflowID)
	if err != nil {
		return -1, err
	}
	maxLayer := -1
	for _, d := range defs {
		if d.Layer > maxLayer {
			maxLayer = d.Layer
		}
	}
	return maxLayer, nil
}
