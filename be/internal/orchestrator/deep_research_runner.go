package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// deepResearchTimeout caps a nested deep-research run; on expiry RunDeepResearch
// stops the run and returns an error rather than blocking the caller forever.
// A var so it is tunable and shrinkable in tests.
var deepResearchTimeout = 30 * time.Minute

// RunDeepResearch starts the deep-research workflow as a project-scoped
// sub-workflow under projectID, waits for it to finish, and returns its `report`
// finding. It implements apirun.DeepResearchRunner for the web_deep_research
// builtin.
//
// The run is detached (started on context.Background()) so a caller stop does
// not cancel it; the wait honors ctx so the tool unblocks promptly if the caller
// goes away. The deep-research definition is global, but execution is
// project-scoped (scope_type=project) — so it skips the ticket re-entrancy guard
// and inherits the caller project's env (Exa/Jina keys) and artifacts.
func (o *Orchestrator) RunDeepResearch(ctx context.Context, projectID, question string) (json.RawMessage, error) {
	res, err := o.Start(context.Background(), RunRequest{
		ProjectID:    projectID,
		WorkflowName: service.DeepResearchWorkflow,
		ScopeType:    "project",
		Instructions: question,
	})
	if err != nil {
		return nil, fmt.Errorf("deep_research: start failed: %w", err)
	}

	deadline := time.NewTimer(deepResearchTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Caller is gone — stop the nested run so it doesn't orphan/pile up.
			_ = o.Stop(res.InstanceID)
			return nil, ctx.Err()
		case <-deadline.C:
			_ = o.Stop(res.InstanceID)
			return nil, fmt.Errorf("deep_research: timed out after %s", deepResearchTimeout)
		case <-ticker.C:
			if o.IsInstanceRunning(res.InstanceID) {
				continue
			}
			return o.readDeepResearchReport(res.InstanceID)
		}
	}
}

// readDeepResearchReport loads the terminal instance and returns its `report`
// finding, erroring when the run did not complete successfully or produced none.
func (o *Orchestrator) readDeepResearchReport(instanceID string) (json.RawMessage, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("deep_research: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID)
	if err != nil {
		return nil, fmt.Errorf("deep_research: load instance: %w", err)
	}
	if wi.Status != model.WorkflowInstanceCompleted && wi.Status != model.WorkflowInstanceProjectCompleted {
		return nil, fmt.Errorf("deep_research: run ended with status %q", wi.Status)
	}

	findings, err := repo.NewFindingRepo(pool, o.clock).GetOwn("workflow_instance", instanceID)
	if err != nil {
		return nil, fmt.Errorf("deep_research: read findings: %w", err)
	}
	// Contract: the deep-research workflow's synthesize agent emits the final
	// report under the finding key "report" (see the seeded workflow definition).
	report, ok := findings["report"]
	if !ok || len(report) == 0 {
		present := make([]string, 0, len(findings))
		for k := range findings {
			present = append(present, k)
		}
		return nil, fmt.Errorf("deep_research: completed but emitted no 'report' finding (present keys: %v)", present)
	}
	return report, nil
}
