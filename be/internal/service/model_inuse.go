package service

import (
	"fmt"
	"strings"
)

// ModelInUseCheck blocks disabling or deleting a model that is still referenced
// by any agent definition, system agent definition, or observer setting (global,
// per-project, or per-workflow).
func (s *ModelService) ModelInUseCheck(id string) error {
	refs, err := s.modelReferences(id, "")
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	return fmt.Errorf("model is in use by: %s", strings.Join(refs, ", "))
}

// ModelInUseCheckForMode blocks clearing a mode's model id while definitions still
// reference the model in that mode. mode is "cli" or "api". Observers spawn in
// cli_interactive mode, so they only strand a cleared "cli" model.
func (s *ModelService) ModelInUseCheckForMode(id, mode string) error {
	refs, err := s.modelReferences(id, mode)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	return fmt.Errorf("model is in use by %s-mode references: %s", mode, strings.Join(refs, ", "))
}

// modelReferences returns human-readable descriptions of every place the model id
// is referenced. mode "" covers all modes (disable/delete); "cli"/"api" restrict to
// references that would be stranded by clearing that mode's model id.
func (s *ModelService) modelReferences(id, mode string) ([]string, error) {
	refs := []string{}

	var defWhere string
	switch mode {
	case "cli":
		defWhere = "(execution_mode != 'api' AND execution_mode != 'script') AND (LOWER(model) = LOWER(?) OR LOWER(low_consumption_model) = LOWER(?))"
	case "api":
		defWhere = "execution_mode = 'api' AND (LOWER(model) = LOWER(?) OR LOWER(low_consumption_model) = LOWER(?))"
	default:
		defWhere = "LOWER(model) = LOWER(?) OR LOWER(low_consumption_model) = LOWER(?)"
	}
	rows, err := s.pool.Query("SELECT project_id, workflow_id, id FROM agent_definitions WHERE "+defWhere, id, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var project, workflow, agent string
		if err := rows.Scan(&project, &workflow, &agent); err != nil {
			rows.Close()
			return nil, err
		}
		if workflow != "" {
			refs = append(refs, project+"/"+workflow+"/"+agent)
		} else {
			refs = append(refs, project+"/"+agent)
		}
	}
	rows.Close()

	var sysWhere string
	switch mode {
	case "cli":
		sysWhere = "execution_mode = 'cli' AND LOWER(model) = LOWER(?)"
	case "api":
		sysWhere = "execution_mode = 'api' AND LOWER(model) = LOWER(?)"
	default:
		sysWhere = "LOWER(model) = LOWER(?)"
	}
	sysRows, err := s.pool.Query("SELECT id FROM system_agent_definitions WHERE "+sysWhere, id)
	if err != nil {
		return nil, err
	}
	for sysRows.Next() {
		var agent string
		if err := sysRows.Scan(&agent); err != nil {
			sysRows.Close()
			return nil, err
		}
		refs = append(refs, "system/"+agent)
	}
	sysRows.Close()

	// Observers spawn cli_interactive, so their refs are stranded by "cli" clearing
	// (and by outright disable/delete), but not by clearing "api".
	if mode != "api" {
		obs, err := s.observerReferences(id)
		if err != nil {
			return nil, err
		}
		refs = append(refs, obs...)
	}
	return refs, nil
}

// observerReferences finds observer model overrides pointing at id: the global and
// per-project observer_model settings (config KV table) and the per-workflow
// workflows.observer_model column.
func (s *ModelService) observerReferences(id string) ([]string, error) {
	refs := []string{}

	cfgRows, err := s.pool.Query("SELECT project_id FROM config WHERE key = ? AND LOWER(value) = LOWER(?)", observerModelKey, id)
	if err != nil {
		return nil, err
	}
	for cfgRows.Next() {
		var project string
		if err := cfgRows.Scan(&project); err != nil {
			cfgRows.Close()
			return nil, err
		}
		if project == "" {
			refs = append(refs, "observer settings")
		} else {
			refs = append(refs, fmt.Sprintf("project %q observer settings", project))
		}
	}
	cfgRows.Close()

	wfRows, err := s.pool.Query("SELECT id FROM workflows WHERE LOWER(observer_model) = LOWER(?)", id)
	if err != nil {
		return nil, err
	}
	for wfRows.Next() {
		var workflow string
		if err := wfRows.Scan(&workflow); err != nil {
			wfRows.Close()
			return nil, err
		}
		refs = append(refs, fmt.Sprintf("workflow %q observer", workflow))
	}
	wfRows.Close()

	return refs, nil
}
