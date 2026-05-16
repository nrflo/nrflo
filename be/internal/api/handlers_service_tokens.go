package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

type serviceTokenResponse struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	DisplayHint string `json:"display_hint"`
	CreatedAt   string `json:"created_at"`
	CreatedBy   string `json:"created_by,omitempty"`
	LastUsedAt  string `json:"last_used_at,omitempty"`
}

// handleListServiceTokens returns every service token across projects, newest
// first. Admin-only. Hashes are never returned.
func (s *Server) handleListServiceTokens(w http.ResponseWriter, r *http.Request) {
	svc := service.NewServiceTokenService(s.pool, s.clock)
	tokens, err := svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]serviceTokenResponse, 0, len(tokens))
	for _, t := range tokens {
		last := ""
		if t.LastUsedAt != nil {
			last = t.LastUsedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		out = append(out, serviceTokenResponse{
			ID:          t.ID,
			ProjectID:   t.ProjectID,
			Name:        t.Name,
			DisplayHint: t.DisplayHint,
			CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
			CreatedBy:   t.CreatedBy,
			LastUsedAt:  last,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateServiceToken mints a new service token. The plaintext token is
// returned exactly once in the response and never persisted in cleartext.
func (s *Server) handleCreateServiceToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Name = strings.TrimSpace(req.Name)
	if req.ProjectID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "project_id and name are required")
		return
	}

	projRepo := repo.NewProjectRepo(s.pool, s.clock)
	proj, err := projRepo.Get(req.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if proj == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	svc := service.NewServiceTokenService(s.pool, s.clock)
	tok, plaintext, err := svc.Create(proj.ID, req.Name, getUserID(r))
	if err != nil {
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "maximum length") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "service_token_create", "service_token", tok.ID, mustMarshal(map[string]string{
		"project_id": tok.ProjectID,
		"name":       tok.Name,
	}))

	if s.wsHub != nil {
		s.wsHub.BroadcastGlobal(ws.NewEvent(ws.EventServiceTokensUpdated, tok.ProjectID, "", "", map[string]interface{}{
			"project_id": tok.ProjectID,
		}))
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token": plaintext,
		"record": serviceTokenResponse{
			ID:          tok.ID,
			ProjectID:   tok.ProjectID,
			Name:        tok.Name,
			DisplayHint: tok.DisplayHint,
			CreatedAt:   tok.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
			CreatedBy:   tok.CreatedBy,
		},
	})
}

// handleDeleteServiceToken revokes a service token by id.
func (s *Server) handleDeleteServiceToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	svc := service.NewServiceTokenService(s.pool, s.clock)
	existing, err := svc.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "service token not found")
		return
	}
	if err := svc.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "service_token_delete", "service_token", id, mustMarshal(map[string]string{
		"project_id": existing.ProjectID,
	}))

	if s.wsHub != nil {
		s.wsHub.BroadcastGlobal(ws.NewEvent(ws.EventServiceTokensUpdated, existing.ProjectID, "", "", map[string]interface{}{
			"project_id": existing.ProjectID,
		}))
	}

	w.WriteHeader(http.StatusNoContent)
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
