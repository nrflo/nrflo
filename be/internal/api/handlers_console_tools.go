package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"be/internal/console"
	"be/internal/model"
)

// consoleToolSummary is one entry in the GET /api/v1/console/tools catalogue.
type consoleToolSummary struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// catalogueForSession resolves sess's tool allowlist: a kind='console_chat'
// row started under a profile restricts the `agent mcp-external` bridge's
// HTTP-mediated tool routes (this file) to that profile's catalogue, exactly
// like the api engine's in-process registry (chat_service.go) — a claude/
// codex t0-decider chat must not regain the full catalogue just because its
// tool calls take the HTTP path instead. A plain kind='console' session, or a
// chat with no profile, gets nil (today's full-catalogue behavior).
func catalogueForSession(sess *model.AgentSession) []string {
	if sess.Kind != model.AgentSessionKindConsoleChat || sess.ConsoleProfile == "" {
		return nil
	}
	profile, err := console.ProfileByName(sess.ConsoleProfile)
	if err != nil {
		return nil
	}
	return profile.Catalogue
}

// handleListConsoleTools serves GET /api/v1/console/tools: the console tool
// catalogue, sorted by name. Auth is enforced in-handler by
// requireConsoleSession (route is `protected`, not `projectAdmin` — a console
// bearer never populates the user context).
func (s *Server) handleListConsoleTools(w http.ResponseWriter, r *http.Request) {
	sess, ok := requireConsoleSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "console session required")
		return
	}

	reg, err := console.BuildRegistry(s.consoleDeps(), catalogueForSession(sess))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	specs := console.Specs(reg)
	tools := make([]consoleToolSummary, 0, len(specs))
	for _, sp := range specs {
		tools = append(tools, consoleToolSummary{Name: sp.Name, Description: sp.Description, InputSchema: sp.InputSchema})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"tools": tools})
}

// handleCallConsoleTool serves POST /api/v1/console/tools/{name}/call. Auth is
// enforced in-handler by requireConsoleSession (route is `protected`).
// Unlisted tool -> 404; malformed body -> 400. Every call is timed and
// audited regardless of outcome.
func (s *Server) handleCallConsoleTool(w http.ResponseWriter, r *http.Request) {
	sess, ok := requireConsoleSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "console session required")
		return
	}

	name := r.PathValue("name")

	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	var body struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	deps := s.consoleDeps()
	reg, err := console.BuildRegistry(deps, catalogueForSession(sess))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projectID := consoleToolProject(r, sess)
	env := console.NewToolEnv(deps, sess.ID, projectID)

	start := s.clock.Now()
	output, isError, callErr := console.Dispatch(r.Context(), reg, env, name, body.Arguments)
	dur := s.clock.Now().Sub(start)

	outcome := "ok"
	switch {
	case errors.Is(callErr, console.ErrToolNotFound):
		outcome = "not_found"
	case callErr != nil:
		outcome = "error"
	case isError:
		outcome = "tool_error"
	}
	appendConsoleToolAudit(s, r, sess, projectID, name, body.Arguments, dur, outcome)

	if errors.Is(callErr, console.ErrToolNotFound) {
		writeError(w, http.StatusNotFound, "unknown tool: "+name)
		return
	}
	if callErr != nil {
		writeError(w, http.StatusInternalServerError, callErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"output":      output,
		"is_error":    isError,
		"duration_ms": dur.Milliseconds(),
	})
}
