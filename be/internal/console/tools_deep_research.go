package console

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// deepResearchPollInterval / deepResearchMaxWait are the tool's own polling
// cadence and upper bound on a blocking run; vars so tests can shrink them.
var (
	deepResearchPollInterval = 3 * time.Second
	deepResearchMaxWait      = 25 * time.Minute
)

// deepResearchHandler implements deep_research: starts the bundled
// deep-research workflow in the global project and blocks (client-side
// polling, not one long request) until it finishes, returning the
// synthesized `report` finding. On ctx cancellation (client disconnect) it
// stops the run so it is not orphaned.
type deepResearchHandler struct{ d Deps }

func (deepResearchHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "deep_research",
		Description: "Run the bundled deep-research workflow and block until it completes, returning the synthesized report.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"question":{"type":"string"},
"context":{"type":"string","description":"Optional caller-supplied context, surfaced to the workflow as external_context"}
},
"required":["question"],
"additionalProperties":false
}`),
	}
}

func (h deepResearchHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Question string `json:"question"`
		Context  string `json:"context"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Question == "" {
		return "question is required", true, nil
	}
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}
	if h.d.WorkflowSvc == nil {
		return missingService("workflow")
	}

	instanceID, err := h.d.Orch.StartWorkflowWithContext(ctx, service.GlobalProjectID, "", service.DeepResearchWorkflow, args.Question, args.Context, "project")
	if err != nil {
		return err.Error(), true, nil
	}

	deadline := time.Now().Add(deepResearchMaxWait)
	for {
		if ctx.Err() != nil {
			_ = h.d.Orch.StopByProject(service.GlobalProjectID, service.DeepResearchWorkflow, instanceID)
			return ctx.Err().Error(), true, nil
		}

		wi, err := loadGuardedInstance(h.d, service.GlobalProjectID, instanceID)
		if err != nil {
			return err.Error(), true, nil
		}
		state, err := h.d.WorkflowSvc.GetStatusByInstance(wi)
		if err != nil {
			return err.Error(), true, nil
		}
		switch fmt.Sprint(state["status"]) {
		case "completed", "project_completed":
			combined := h.d.WorkflowSvc.BuildCombinedFindings(wi)
			report, err := service.ExtractReport(combined, instanceID)
			if err != nil {
				return err.Error(), true, nil
			}
			return report, false, nil
		case "failed":
			return fmt.Sprintf("deep-research run %s failed; inspect it with workflow_get(instance_id=%s)", instanceID, instanceID), true, nil
		}
		if time.Now().After(deadline) {
			return fmt.Sprintf("deep-research run %s still running after %s; poll workflow_get with instance_id=%s", instanceID, deepResearchMaxWait, instanceID), true, nil
		}

		select {
		case <-ctx.Done():
			_ = h.d.Orch.StopByProject(service.GlobalProjectID, service.DeepResearchWorkflow, instanceID)
			return ctx.Err().Error(), true, nil
		case <-time.After(deepResearchPollInterval):
		}
	}
}
