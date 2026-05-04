package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"be/internal/manifest/config"
	"be/internal/configeditor"
	"be/internal/repo"
	"be/internal/ws"
)

// buildConfigEditor constructs a configeditor.Service for the given project.
// Returns a non-nil error and writes an error response if customer_config_dir is not configured.
func (s *Server) buildConfigEditor(w http.ResponseWriter, projectID string) (*configeditor.Service, bool) {
	configDir, err := s.pool.GetProjectConfig(projectID, "customer_config_dir")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if configDir == "" {
		writeError(w, http.StatusBadRequest,
			"customer_config_dir is not configured for this project; set it in project settings")
		return nil, false
	}

	manifest, err := config.Load(configDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load manifest: %s", err))
		return nil, false
	}

	r := repo.NewConfigVersionRepo(s.pool.DB, s.clock)
	svc := configeditor.NewService(configDir, manifest, r, s.clock)
	return svc, true
}

// validateRelPathParam checks that a file path (from URL path or query) is safe.
func validateRelPathParam(file string) error {
	if file == "" {
		return fmt.Errorf("file path must not be empty")
	}
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path must be relative")
	}
	clean := filepath.Clean(file)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file path must not traverse parent directories")
	}
	return nil
}

func (s *Server) handleListConfigFiles(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	svc, ok := s.buildConfigEditor(w, projectID)
	if !ok {
		return
	}
	files, err := svc.List(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if files == nil {
		files = []configeditor.FileMeta{}
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) handleGetConfigFile(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	file := r.PathValue("file")
	if err := validateRelPathParam(file); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, ok := s.buildConfigEditor(w, projectID)
	if !ok {
		return
	}
	content, err := svc.Get(projectID, file)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ver, _ := repo.NewConfigVersionRepo(s.pool.DB, s.clock).LatestVersion(projectID, file)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file":    file,
		"content": string(content),
		"version": ver,
	})
}

func (s *Server) handlePutConfigFile(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	file := r.PathValue("file")
	if err := validateRelPathParam(file); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	svc, ok := s.buildConfigEditor(w, projectID)
	if !ok {
		return
	}
	if err := svc.Put(projectID, file, "", body); err != nil {
		var ve *configeditor.ValidationError
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":  ve.Error(),
				"fields": ve.Fields,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ver, _ := repo.NewConfigVersionRepo(s.pool.DB, s.clock).LatestVersion(projectID, file)
	s.wsHub.Broadcast(&ws.Event{
		Type:      ws.EventConfigFileUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"file": file, "version": ver},
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file":    file,
		"version": ver,
	})
}

func (s *Server) handleGetConfigHistory(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	file := r.PathValue("file")
	if err := validateRelPathParam(file); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, ok := s.buildConfigEditor(w, projectID)
	if !ok {
		return
	}
	versions, err := svc.History(projectID, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if versions == nil {
		versions = nil // keep nil; JSON encodes as null which is fine for empty history
	}
	writeJSON(w, http.StatusOK, versions)
}

type configRollbackRequest struct {
	Version int `json:"version"`
}

func (s *Server) handleRollbackConfig(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	file := r.PathValue("file")
	if err := validateRelPathParam(file); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Accept version from body or query param.
	var toVersion int
	var req configRollbackRequest
	if err := readJSON(r, &req); err == nil && req.Version > 0 {
		toVersion = req.Version
	} else if v := r.URL.Query().Get("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "version must be a positive integer")
			return
		}
		toVersion = n
	}
	if toVersion <= 0 {
		writeError(w, http.StatusBadRequest, "version is required")
		return
	}

	svc, ok := s.buildConfigEditor(w, projectID)
	if !ok {
		return
	}
	if err := svc.Rollback(projectID, file, "", toVersion); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ver, _ := repo.NewConfigVersionRepo(s.pool.DB, s.clock).LatestVersion(projectID, file)
	s.wsHub.Broadcast(&ws.Event{
		Type:      ws.EventConfigFileUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"file": file, "version": ver},
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file":    file,
		"version": ver,
	})
}
