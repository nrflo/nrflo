package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_builtin"
)

// availableTool describes a tool an agent can be granted via its tools CSV.
// It backs the per-agent tools picker in the workflow editor.
type availableTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"` // "builtin" | "python"
	Mandatory   bool   `json:"mandatory"`
}

// availableAgentTools returns the builtin tools plus the project's python
// (kind=tool) scripts. Baseline (always-granted) builtins are flagged
// mandatory — the spawner always grants them to socket-completion agents
// regardless of the tools CSV.
func (s *Server) availableAgentTools(projectID string) []availableTool {
	mandatory := make(map[string]bool)
	for _, n := range tools_builtin.BaselineToolNames() {
		mandatory[n] = true
	}

	builtins := tools_builtin.Builtins()
	out := make([]availableTool, 0, len(builtins))
	for name, h := range builtins {
		out = append(out, availableTool{
			Name:        name,
			Description: h.Spec().Description,
			Source:      "builtin",
			Mandatory:   mandatory[name],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	if projectID != "" {
		pyTools, err := service.NewPythonScriptService(s.pool, s.clock).ListTools(projectID)
		if err == nil {
			py := make([]availableTool, 0, len(pyTools))
			for _, row := range pyTools {
				py = append(py, availableTool{
					Name:        row.Name,
					Description: row.ToolDescription,
					Source:      "python",
				})
			}
			sort.Slice(py, func(i, j int) bool { return py[i].Name < py[j].Name })
			out = append(out, py...)
		}
	}
	return out
}

// handleListAvailableTools serves GET /api/v1/available-tools, the source for
// the per-agent tools picker. Builtins are global; python tools are resolved
// from the X-Project scope.
func (s *Server) handleListAvailableTools(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	writeJSON(w, http.StatusOK, s.availableAgentTools(projectID))
}

// toolsCSVWarnings returns a warning for each non-"*" pattern in the tools CSV
// that matches no known tool. Warn-only: a pattern may legitimately target a
// tool created later, so this never blocks a save.
func toolsCSVWarnings(toolsCSV string, available []availableTool) []string {
	var warnings []string
	for _, raw := range strings.Split(toolsCSV, ",") {
		pat := strings.TrimSpace(raw)
		if pat == "" || pat == "*" {
			continue
		}
		matched := false
		for _, t := range available {
			if apirun.MatchName(pat, t.Name) {
				matched = true
				break
			}
		}
		if !matched {
			warnings = append(warnings, fmt.Sprintf("tools pattern %q matches no known tool", pat))
		}
	}
	return warnings
}
