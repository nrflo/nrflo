package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun"
)

type registerToolEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Endpoint    string          `json:"endpoint"`
	InputSchema json.RawMessage `json:"input_schema"`
	AuthMethod  string          `json:"auth_method,omitempty"`
	AuthRef     *string         `json:"auth_ref,omitempty"`
	TimeoutSec  int             `json:"timeout_sec,omitempty"`
}

type registerToolsResponse struct {
	ToolsRegistered   int      `json:"tools_registered"`
	ToolsPruned       int      `json:"tools_pruned"`
	ToolsSkippedInUse []string `json:"tools_skipped_in_use"`
}

// handleRegisterToolDefinitions handles POST /api/v1/tool-definitions/register.
// Requires bearer auth via NRFLO_REGISTER_TOKEN env var (503 when unset).
// Idempotently upserts a manifest of global tool definitions and prunes stale entries
// not referenced by any agent_definitions.tools pattern.
func (s *Server) handleRegisterToolDefinitions(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("NRFLO_REGISTER_TOKEN")
	if token == "" {
		writeError(w, http.StatusServiceUnavailable, "registration endpoint not configured: NRFLO_REGISTER_TOKEN not set")
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "bearer token required")
		return
	}
	provided := strings.TrimPrefix(authHeader, "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var raw struct {
		Tools json.RawMessage `json:"tools"`
	}
	if err := readJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(raw.Tools) == 0 || string(raw.Tools) == "null" {
		writeError(w, http.StatusBadRequest, "tools is required")
		return
	}

	var entries []registerToolEntry
	if err := json.Unmarshal(raw.Tools, &entries); err != nil {
		writeError(w, http.StatusBadRequest, "tools must be an array")
		return
	}

	if err := validateRegisterEntries(entries); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	toolDefRepo := repo.NewToolDefinitionRepo(s.pool, s.clock)
	for _, e := range entries {
		am := e.AuthMethod
		if am == "" {
			am = "none"
		}
		def := &model.ToolDefinition{
			Name:        e.Name,
			Description: e.Description,
			Endpoint:    e.Endpoint,
			InputSchema: e.InputSchema,
			AuthMethod:  am,
			AuthRef:     e.AuthRef,
			TimeoutSec:  e.TimeoutSec,
		}
		if err := toolDefRepo.UpsertByName(def); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	payloadNames := make(map[string]bool, len(entries))
	for _, e := range entries {
		payloadNames[strings.ToLower(e.Name)] = true
	}

	agentDefRepo := repo.NewAgentDefinitionRepo(s.pool, s.clock)
	csvs, err := agentDefRepo.AllToolsCSVs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var patterns []string
	for _, csv := range csvs {
		for _, p := range strings.Split(csv, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				patterns = append(patterns, p)
			}
		}
	}

	existingGlobals, err := toolDefRepo.ListGlobalRegistered()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pruned := 0
	skipped := []string{}
	for _, existing := range existingGlobals {
		if payloadNames[strings.ToLower(existing.Name)] {
			continue
		}
		protected := false
		for _, pat := range patterns {
			if apirun.MatchName(pat, existing.Name) {
				protected = true
				break
			}
		}
		if protected {
			skipped = append(skipped, existing.Name)
			continue
		}
		if err := toolDefRepo.DeleteByName(existing.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		pruned++
	}

	logger.Info(r.Context(), "tool registration complete",
		"registered", len(entries), "pruned", pruned, "skipped_in_use", len(skipped))

	writeJSON(w, http.StatusOK, registerToolsResponse{
		ToolsRegistered:   len(entries),
		ToolsPruned:       pruned,
		ToolsSkippedInUse: skipped,
	})
}
