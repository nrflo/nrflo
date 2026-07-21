package api

import (
	"errors"
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/service"
	"be/internal/types"
)

// handleListConsoleSkills returns the resolved project's discovered
// .claude/skills for the console chat "/" suggestion dropdown.
// GET /api/v1/console/skills
//
// Route is `protected`, not `projectAdmin` — auth is enforced in-handler,
// mirroring /console/tools (requireConsoleSession). Unlike consoleToolProject,
// a console/console_chat bearer is pinned unconditionally to its own
// session's project (X-Project/?project= is ignored) so a chat bearer can
// never read another project's skills. Non-console callers keep today's
// requireProjectAdmin semantics: admin user, or a service token that is
// global or whose project matches the resolved project.
func (s *Server) handleListConsoleSkills(w http.ResponseWriter, r *http.Request) {
	var projectID string
	if sess, ok := requireConsoleSession(r); ok {
		projectID = sess.ProjectID
	} else if u := getUser(r); u != nil && u.Role == model.UserRoleAdmin {
		projectID = getProjectID(r)
	} else if sp := getServicePrincipal(r); sp != nil {
		if sp.Global || (getProjectID(r) != "" && strings.EqualFold(getProjectID(r), sp.ProjectID)) {
			projectID = getProjectID(r)
		} else {
			writeError(w, http.StatusForbidden, "project scope required")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}
	skills, err := s.consoleChat.ListSkills(projectID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, types.ConsoleSkillsResponse{ProjectID: projectID, Skills: skills})
}
