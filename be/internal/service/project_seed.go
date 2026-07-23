package service

import (
	"context"
	"strings"

	"be/internal/logger"
)

// classicWorkflowPhase is one agent_definitions row to seed for a classic
// workflow. role must be a TierMap key and a default_templates id (they
// share the same names) so the seeded def is both tiered and prompted.
type classicWorkflowPhase struct {
	role  string
	layer int
}

type classicWorkflow struct {
	id          string
	description string
	phases      []classicWorkflowPhase
}

// classicWorkflows mirrors the "Workflows" table in the root CLAUDE.md:
// feature/bugfix/hotfix/docs/refactor with their per-phase layer order.
var classicWorkflows = []classicWorkflow{
	{id: "feature", description: "New features (full TDD)", phases: []classicWorkflowPhase{
		{"setup-analyzer", 0}, {"test-writer", 1}, {"implementor", 2}, {"qa-verifier", 3}, {"doc-updater", 4},
	}},
	{id: "bugfix", description: "Bug fixes", phases: []classicWorkflowPhase{
		{"setup-analyzer", 0}, {"implementor", 1}, {"qa-verifier", 2},
	}},
	{id: "hotfix", description: "Urgent fixes", phases: []classicWorkflowPhase{
		{"implementor", 0},
	}},
	{id: "docs", description: "Documentation only", phases: []classicWorkflowPhase{
		{"setup-analyzer", 0}, {"doc-updater", 1},
	}},
	{id: "refactor", description: "Code refactoring", phases: []classicWorkflowPhase{
		{"setup-analyzer", 0}, {"implementor", 1}, {"qa-verifier", 2},
	}},
}

// seedTieredWorkflows materializes the classic workflow set for a newly
// created project, with each phase's model/reasoning_effort/system_template_id
// taken from TierMap so fresh projects are born tiered. The hotfix
// implementor is the one exception (IsHotfixImplementor): it gets the map's
// implementor model but no forced effort/template, matching "urgency over
// cost". Best-effort/logged, like seedSpecImportWorkflow — failures never
// fail project create.
func (s *ProjectService) seedTieredWorkflows(projectID, now string) {
	templates, err := s.loadClassicTemplates()
	if err != nil {
		logger.Warn(context.Background(), "seedTieredWorkflows: failed to load templates", "project_id", projectID, "err", err)
		return
	}

	for _, wf := range classicWorkflows {
		_, err := s.pool.Exec(`
			INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
			VALUES (?, ?, ?, 'ticket', '[]', ?, ?)`,
			wf.id, projectID, wf.description, now, now,
		)
		if err != nil {
			logger.Warn(context.Background(), "seedTieredWorkflows: failed to insert workflow", "project_id", projectID, "workflow_id", wf.id, "err", err)
			continue
		}

		for _, phase := range wf.phases {
			target := TierMap[phase.role]
			systemTemplateID := target.SystemTemplateID
			tools := ""
			if target.GrantsDelegation && !IsHotfixImplementor(wf.id, phase.role) {
				tools = delegationToolsCSV
			}

			// The hotfix implementor is the one exception: "urgency over
			// cost" keeps its explicit seed model override (sonnet-5), no
			// tier, no forced effort/template.
			var model string
			var tierVal interface{}
			if IsHotfixImplementor(wf.id, phase.role) {
				model, systemTemplateID = "sonnet-5", ""
			} else {
				tierVal = target.Tier
			}

			_, err := s.pool.Exec(`
				INSERT OR IGNORE INTO agent_definitions
					(id, project_id, workflow_id, model, timeout, prompt, layer, reasoning_effort, system_template_id, tools, tier, created_at, updated_at)
				VALUES (?, ?, ?, ?, 20, ?, ?, NULL, ?, ?, ?, ?, ?)`,
				phase.role, projectID, wf.id, model, templates[phase.role], phase.layer, systemTemplateID, tools, tierVal, now, now,
			)
			if err != nil {
				logger.Warn(context.Background(), "seedTieredWorkflows: failed to insert agent_definition", "project_id", projectID, "workflow_id", wf.id, "def_id", phase.role, "err", err)
			}
		}
	}
}

// loadClassicTemplates reads the readonly default_templates prompt bodies
// for the five classic phase roles, keyed by role/template id.
func (s *ProjectService) loadClassicTemplates() (map[string]string, error) {
	ids := []string{"setup-analyzer", "test-writer", "implementor", "qa-verifier", "doc-updater"}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.pool.Query("SELECT id, template FROM default_templates WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string, len(ids))
	for rows.Next() {
		var id, template string
		if err := rows.Scan(&id, &template); err != nil {
			return nil, err
		}
		out[id] = template
	}
	return out, rows.Err()
}
