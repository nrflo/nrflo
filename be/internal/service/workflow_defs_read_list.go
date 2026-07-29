package service

import (
	"strings"

	"be/internal/model"
)

// ListWorkflowDefs returns the selectable workflow definitions for a project:
// the project's own definitions unioned with all global definitions. On a name
// collision a project-local definition shadows the global one (local
// precedence). Execution of a global definition still runs project-scoped; only
// the definition is shared. Reserved (`__name__`) workflows are excluded.
func (s *WorkflowService) ListWorkflowDefs(projectID string) (map[string]WorkflowDef, error) {
	localMetas, err := s.queryWorkflowMetas(`LOWER(project_id) = LOWER(?)`, projectID)
	if err != nil {
		return nil, err
	}
	localAgents, err := s.listAgentDefsForProject(projectID)
	if err != nil {
		return nil, err
	}
	localByWF := groupAgentsByWorkflow(localAgents)

	result := make(map[string]WorkflowDef)

	// Global definitions go in first so a same-named local definition overwrites
	// them below (local precedence). Skipped when the caller already is the
	// global namespace, since localMetas then already holds the global rows.
	if !strings.EqualFold(projectID, GlobalProjectID) {
		globalMetas, err := s.queryWorkflowMetas(`is_global = 1 AND LOWER(project_id) = LOWER(?)`, GlobalProjectID)
		if err != nil {
			return nil, err
		}
		globalAgents, err := s.listAgentDefsForProject(GlobalProjectID)
		if err != nil {
			return nil, err
		}
		globalByWF := groupAgentsByWorkflow(globalAgents)
		for _, m := range globalMetas {
			if IsHiddenWorkflowName(m.id) {
				continue
			}
			result[m.id] = buildWorkflowDef(m, globalByWF[m.id])
		}
	}

	for _, m := range localMetas {
		if IsHiddenWorkflowName(m.id) {
			continue
		}
		result[m.id] = buildWorkflowDef(m, localByWF[m.id])
	}

	return result, nil
}

// queryWorkflowMetas runs the shared workflow-def SELECT with a WHERE clause and
// scans the rows into metas.
func (s *WorkflowService) queryWorkflowMetas(where string, args ...any) ([]wfMeta, error) {
	rows, err := s.pool.Query(`SELECT `+workflowDefCols+` FROM workflows WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metas []wfMeta
	for rows.Next() {
		m, err := scanWFMeta(rows)
		if err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// groupAgentsByWorkflow buckets agent definitions by their workflow id.
func groupAgentsByWorkflow(defs []*model.AgentDefinition) map[string][]*model.AgentDefinition {
	byWF := make(map[string][]*model.AgentDefinition)
	for _, ad := range defs {
		byWF[ad.WorkflowID] = append(byWF[ad.WorkflowID], ad)
	}
	return byWF
}
