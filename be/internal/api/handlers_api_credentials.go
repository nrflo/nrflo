package api

import (
	"net/http"
	"strings"

	"be/internal/artifact"
	"be/internal/model"
	"be/internal/repo"
)

const literalSecretRedacted = "literal:***"

type apiCredentialCreateRequest struct {
	ID        string  `json:"id"`
	Provider  string  `json:"provider"`
	ProjectID *string `json:"project_id,omitempty"`
	SecretRef string  `json:"secret_ref"`
}

type apiCredentialUpdateRequest struct {
	Provider  *string `json:"provider,omitempty"`
	ProjectID *string `json:"project_id,omitempty"`
	SecretRef *string `json:"secret_ref,omitempty"`
}

// redactCredential returns a shallow copy with literal:* secret_ref values
// replaced by literal:***. Other forms (env:, file:) are kept as-is — they
// are references, not the secret itself.
func redactCredential(cred *model.APICredential) *model.APICredential {
	if cred == nil {
		return nil
	}
	out := *cred
	out.SecretRef = artifact.RedactSecretRef(out.SecretRef)
	return &out
}

func (s *Server) handleListAPICredentials(w http.ResponseWriter, r *http.Request) {
	r0 := repo.NewAPICredentialRepo(s.pool, s.clock)
	creds, err := r0.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]*model.APICredential, 0, len(creds))
	for _, c := range creds {
		out = append(out, redactCredential(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateAPICredential(w http.ResponseWriter, r *http.Request) {
	var req apiCredentialCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.SecretRef == "" {
		writeError(w, http.StatusBadRequest, "secret_ref is required")
		return
	}
	if !isValidSecretRef(req.SecretRef) {
		writeError(w, http.StatusBadRequest, "secret_ref must start with env:, file:, or literal:")
		return
	}

	cred := &model.APICredential{
		ID:        req.ID,
		Provider:  req.Provider,
		ProjectID: req.ProjectID,
		SecretRef: req.SecretRef,
	}

	r0 := repo.NewAPICredentialRepo(s.pool, s.clock)
	if err := r0.Create(cred); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "api credential already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	saved, err := r0.Get(cred.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, redactCredential(saved))
}

func (s *Server) handleGetAPICredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r0 := repo.NewAPICredentialRepo(s.pool, s.clock)
	cred, err := r0.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, redactCredential(cred))
}

func (s *Server) handleUpdateAPICredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req apiCredentialUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SecretRef != nil {
		if *req.SecretRef == "" {
			writeError(w, http.StatusBadRequest, "secret_ref must not be empty")
			return
		}
		if *req.SecretRef == literalSecretRedacted {
			writeError(w, http.StatusBadRequest, "secret_ref is the redacted placeholder; provide a real value")
			return
		}
		if !isValidSecretRef(*req.SecretRef) {
			writeError(w, http.StatusBadRequest, "secret_ref must start with env:, file:, or literal:")
			return
		}
	}

	r0 := repo.NewAPICredentialRepo(s.pool, s.clock)
	fields := &repo.APICredentialUpdateFields{
		Provider:  req.Provider,
		ProjectID: req.ProjectID,
		SecretRef: req.SecretRef,
	}
	if err := r0.Update(id, fields); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cred, err := r0.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, redactCredential(cred))
}

func (s *Server) handleDeleteAPICredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r0 := repo.NewAPICredentialRepo(s.pool, s.clock)
	if err := r0.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func isValidSecretRef(s string) bool {
	return strings.HasPrefix(s, "env:") || strings.HasPrefix(s, "file:") || strings.HasPrefix(s, "literal:")
}
