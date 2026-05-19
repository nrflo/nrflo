package api

import (
	"net/http"

	"be/internal/service"
)

type menuSetting struct {
	key string
	def bool
}

var menuSettings = []menuSetting{
	{"menu_new_ticket", false},
	{"menu_import_spec", false},
	{"menu_git", true},
	{"menu_chain_executions", true},
	{"menu_schedules", false},
	{"menu_workflow_chains", false},
	{"menu_python_scripts", false},
	{"menu_documentation", true},
	{"menu_errors", false},
	{"menu_agent_sessions", false},
}

func boolWithDefault(svc *service.GlobalSettingsService, key string, def bool) (bool, error) {
	val, err := svc.Get(key)
	if err != nil {
		return false, err
	}
	if val == "" {
		return def, nil
	}
	return val == "true", nil
}

type menuPatchFields struct {
	MenuNewTicket       *bool `json:"menu_new_ticket"`
	MenuImportSpec      *bool `json:"menu_import_spec"`
	MenuGit             *bool `json:"menu_git"`
	MenuChainExecutions *bool `json:"menu_chain_executions"`
	MenuSchedules       *bool `json:"menu_schedules"`
	MenuWorkflowChains  *bool `json:"menu_workflow_chains"`
	MenuPythonScripts   *bool `json:"menu_python_scripts"`
	MenuDocumentation   *bool `json:"menu_documentation"`
	MenuErrors          *bool `json:"menu_errors"`
	MenuAgentSessions   *bool `json:"menu_agent_sessions"`
}

func applyMenuToggles(fields menuPatchFields, svc *service.GlobalSettingsService, w http.ResponseWriter) error {
	toggles := []struct {
		ptr *bool
		key string
	}{
		{fields.MenuNewTicket, "menu_new_ticket"},
		{fields.MenuImportSpec, "menu_import_spec"},
		{fields.MenuGit, "menu_git"},
		{fields.MenuChainExecutions, "menu_chain_executions"},
		{fields.MenuSchedules, "menu_schedules"},
		{fields.MenuWorkflowChains, "menu_workflow_chains"},
		{fields.MenuPythonScripts, "menu_python_scripts"},
		{fields.MenuDocumentation, "menu_documentation"},
		{fields.MenuErrors, "menu_errors"},
		{fields.MenuAgentSessions, "menu_agent_sessions"},
	}
	for _, t := range toggles {
		if t.ptr == nil {
			continue
		}
		val := "false"
		if *t.ptr {
			val = "true"
		}
		if err := svc.Set(t.key, val); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return err
		}
	}
	return nil
}
