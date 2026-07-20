package service

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// TieringService builds the dry-run re-tier report and applies confirmed
// changes, both driven by TierMap (tiering.go).
type TieringService struct {
	pool     *db.Pool
	clock    clock.Clock
	modelSvc *ModelService
}

// NewTieringService constructs a TieringService.
func NewTieringService(pool *db.Pool, clk clock.Clock, modelSvc *ModelService) *TieringService {
	return &TieringService{pool: pool, clock: clk, modelSvc: modelSvc}
}

// tieringDefRowRaw is one agent_definitions row read for classification.
type tieringDefRowRaw struct {
	id, workflowID, model, nodeRole string
	effort                          sql.NullString
	consultant                      bool
}

// BuildReport lists, per project, every agent_definitions row that
// classifies to a TierMap role: current vs recommended model/effort, the
// customized flag, a skip reason when the def is ineligible for apply, and
// an estimated monthly cost delta where session cost data is available.
func (s *TieringService) BuildReport() (*types.TieringReport, error) {
	projects, err := NewProjectService(s.pool, s.clock).List()
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	now := s.clock.Now().UTC()
	report := &types.TieringReport{Projects: make([]types.TieringProjectReport, 0, len(projects))}

	for _, p := range projects {
		raws, err := s.loadDefs(p.ID)
		if err != nil {
			return nil, err
		}

		var defs []types.TieringDefRow
		var totalDelta float64
		var haveTotal bool

		for _, raw := range raws {
			role, ok := ClassifyRole(raw.workflowID, raw.id)
			if !ok {
				continue
			}
			target := TierMap[role]
			row := types.TieringDefRow{
				WorkflowID:          raw.workflowID,
				DefID:               raw.id,
				Role:                role,
				CurrentModel:        raw.model,
				CurrentEffort:       raw.effort.String,
				RecommendedModel:    target.RecommendedModel,
				RecommendedEffort:   target.RecommendedEffort,
				RecommendedTemplate: target.SystemTemplateID,
				Customized:          isTierCustomized(raw.model, target),
				IsWorker:            target.IsWorker,
			}
			row.SkipReason = tieringSkipReason(raw, role, row.Customized)

			if row.SkipReason == "" || row.SkipReason == "customized" {
				delta, err := estimateMonthlyDelta(s.pool, s.modelSvc, p.ID, raw.id, raw.model, target.RecommendedModel, now)
				if err != nil {
					return nil, err
				}
				if delta.HasUsage {
					d := delta.Delta
					row.EstMonthlyDelta = &d
					totalDelta += d
					haveTotal = true
				}
			}

			defs = append(defs, row)
		}

		pr := types.TieringProjectReport{ProjectID: p.ID, ProjectName: p.Name, Defs: defs}
		if haveTotal {
			pr.EstMonthlyDelta = &totalDelta
		}
		report.Projects = append(report.Projects, pr)
	}

	report.Markdown = renderTieringMarkdown(report.Projects)
	return report, nil
}

// tieringSkipReason applies the apply-time eligibility rules (consultant,
// hotfix implementor, non-static node role, customized model) so the
// dry-run report and the apply flow always agree on what would be skipped.
func tieringSkipReason(raw tieringDefRowRaw, role string, customized bool) string {
	switch {
	case raw.consultant:
		return "consultant"
	case IsHotfixImplementor(raw.workflowID, role):
		return "hotfix"
	case raw.nodeRole != "static":
		return "non-static"
	case customized:
		return "customized"
	default:
		return ""
	}
}

// renderTieringMarkdown builds a markdown-renderable summary of the report
// for the admin UI / dry-run consumers that prefer prose over raw JSON.
func renderTieringMarkdown(projects []types.TieringProjectReport) string {
	var b strings.Builder
	b.WriteString("# Tiering Report\n\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "## %s (%s)\n\n", p.ProjectName, p.ProjectID)
		if len(p.Defs) == 0 {
			b.WriteString("_No tiered defs found._\n\n")
			continue
		}
		b.WriteString("| Workflow | Def | Role | Current | Recommended | Customized | Skip Reason | Est. Monthly Delta |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|\n")
		for _, d := range p.Defs {
			delta := "n/a"
			if d.EstMonthlyDelta != nil {
				delta = fmt.Sprintf("$%.2f", *d.EstMonthlyDelta)
			}
			skip := d.SkipReason
			if skip == "" {
				skip = "-"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %t | %s | %s |\n",
				d.WorkflowID, d.DefID, d.Role, d.CurrentModel, d.RecommendedModel, d.Customized, skip, delta)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *TieringService) loadDefs(projectID string) ([]tieringDefRowRaw, error) {
	rows, err := s.pool.Query(`
		SELECT id, workflow_id, model, reasoning_effort, consultant, node_role
		FROM agent_definitions WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent definitions for project %s: %w", projectID, err)
	}
	defer rows.Close()

	var out []tieringDefRowRaw
	for rows.Next() {
		var raw tieringDefRowRaw
		if err := rows.Scan(&raw.id, &raw.workflowID, &raw.model, &raw.effort, &raw.consultant, &raw.nodeRole); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
