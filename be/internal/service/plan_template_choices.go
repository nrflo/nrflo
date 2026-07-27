package service

import (
	"encoding/json"

	"be/internal/clock"
	"be/internal/db"
)

// String renders a tier for caller-facing payloads (plan template choices).
func (t ModelTier) String() string {
	switch t {
	case ModelTierPremium:
		return "premium"
	case ModelTierMid:
		return "mid"
	default:
		return "cheap"
	}
}

// PlanTemplateChoice is the caller-facing view of one fanout_template a plan
// node may bind to: the exact string a node's `template` field takes, what the
// template does, and the cost tier it resolves to. Tier is what
// EnforcePremiumWorkerCap counts against dynwf_max_premium_workers — a node has
// no model/effort field of its own, so binding a template is the only way to
// choose either.
type PlanTemplateChoice struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model"`
	Tier        string `json:"tier"`
}

// PlanTemplateChoices returns the templates a plan for (projectID, workflowID)
// may bind nodes to — the same install-usable library the planner is shown
// (EnabledTemplates drops templates whose model is disabled, whose CLI is
// absent, or which need api mode while it is off). Callers that hand-write a
// manifest need this: an unknown `template` is rejected by ValidatePlanManifest.
func PlanTemplateChoices(pool *db.Pool, clk clock.Clock, projectID, workflowID string) ([]PlanTemplateChoice, error) {
	templates, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	modelSvc := NewModelService(pool, clk)
	enabled := EnabledTemplates(pool, clk, templates)

	out := make([]PlanTemplateChoice, 0, len(enabled))
	for _, t := range enabled {
		c := PlanTemplateChoice{ID: t.ID, Description: t.Description, Model: t.Model, Tier: ModelTierMid.String()}
		if row, rerr := modelSvc.Get(t.Model); rerr == nil {
			c.Tier = PlanModelTierClass(row).String()
		}
		out = append(out, c)
	}
	return out, nil
}

// PlanTemplateChoicesJSON is the marshalled form both GetSubworkflow
// implementations (orchestrator, console) attach to a plan-boundary poll
// result. Best-effort: an unreadable library yields nil rather than failing the
// poll, since the manifest and revision are still actionable without it.
func PlanTemplateChoicesJSON(pool *db.Pool, clk clock.Clock, projectID, workflowID string) json.RawMessage {
	choices, err := PlanTemplateChoices(pool, clk, projectID, workflowID)
	if err != nil || len(choices) == 0 {
		return nil
	}
	raw, err := json.Marshal(choices)
	if err != nil {
		return nil
	}
	return raw
}
