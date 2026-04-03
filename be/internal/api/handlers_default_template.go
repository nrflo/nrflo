package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleListDefaultTemplates returns all default templates
func (s *Server) handleListDefaultTemplates(w http.ResponseWriter, r *http.Request) {
	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	templates, err := svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, templates)
}

// handleCreateDefaultTemplate creates a new default template
func (s *Server) handleCreateDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	var req types.DefaultTemplateCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Template == "" {
		writeError(w, http.StatusBadRequest, "template is required")
		return
	}

	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	tmpl, err := svc.Create(&req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventDefaultTemplateCreated, "", "", "", map[string]interface{}{
			"template_id": tmpl.ID,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

// handleGetDefaultTemplate returns a single default template
func (s *Server) handleGetDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	tmpl, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tmpl)
}

// handleUpdateDefaultTemplate updates a default template
func (s *Server) handleUpdateDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req types.DefaultTemplateUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	if err := svc.Update(id, &req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "cannot modify name") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventDefaultTemplateUpdated, "", "", "", map[string]interface{}{
			"template_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteDefaultTemplate deletes a default template
func (s *Server) handleDeleteDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	if err := svc.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "readonly") {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventDefaultTemplateDeleted, "", "", "", map[string]interface{}{
			"template_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleRestoreDefaultTemplate restores a readonly template to its original text
func (s *Server) handleRestoreDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewDefaultTemplateService(s.pool, s.clock)

	if err := svc.Restore(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "non-readonly") || strings.Contains(err.Error(), "no default") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventDefaultTemplateUpdated, "", "", "", map[string]interface{}{
			"template_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}
