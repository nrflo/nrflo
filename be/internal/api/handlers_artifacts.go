package api

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"be/internal/service"
	"be/internal/types"
)

// handleStageUpload accepts a multipart upload and stages it in a temp directory.
// POST /api/v1/artifact-uploads
func (s *Server) handleStageUpload(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}
	userID := getUserID(r)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "multipart parse error: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	svc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	uploadID, err := svc.StageUpload(r.Context(), projectID, userID, header.Filename, file, header.Size, contentType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, types.ArtifactUploadResponse{
		UploadID:    uploadID,
		Name:        header.Filename,
		SizeBytes:   header.Size,
		ContentType: contentType,
	})
}

// handleCancelUpload removes a staged upload.
// DELETE /api/v1/artifact-uploads/{upload_id}
func (s *Server) handleCancelUpload(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}
	uploadID := r.PathValue("upload_id")
	if uploadID == "" {
		writeError(w, http.StatusBadRequest, "upload_id required")
		return
	}

	svc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	meta, err := svc.ReadUploadMeta(uploadID)
	if err != nil {
		writeError(w, http.StatusNotFound, "upload not found")
		return
	}

	userID := getUserID(r)
	if meta.ProjectID != projectID || meta.UserID != userID {
		writeError(w, http.StatusForbidden, "not authorized to cancel this upload")
		return
	}

	if err := svc.CancelUpload(r.Context(), uploadID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "upload_id": uploadID})
}

// handleListArtifacts returns all artifacts for a workflow instance.
// GET /api/v1/workflow-instances/{iid}/artifacts
func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	if iid == "" {
		writeError(w, http.StatusBadRequest, "workflow instance ID required")
		return
	}

	svc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	artifacts, err := svc.List(r.Context(), iid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dtos := make([]types.ArtifactDTO, 0, len(artifacts))
	for _, a := range artifacts {
		dtos = append(dtos, types.ArtifactDTO{
			ID:                 a.ID,
			ProjectID:          a.ProjectID,
			WorkflowInstanceID: a.WorkflowInstanceID,
			Name:               a.Name,
			Type:               a.Type,
			SizeBytes:          a.SizeBytes,
			ContentType:        a.ContentType,
			Source:             a.Source,
			CreatedBySession:   a.CreatedBySession,
			CreatedAt:          a.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, dtos)
}

// handleDownloadArtifact streams an artifact to the client.
// GET /api/v1/artifacts/{aid}/download
func (s *Server) handleDownloadArtifact(w http.ResponseWriter, r *http.Request) {
	aid := r.PathValue("aid")
	if aid == "" {
		writeError(w, http.StatusBadRequest, "artifact ID required")
		return
	}

	svc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	a, err := svc.Get(r.Context(), aid)
	if err != nil || a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	rc, err := svc.Open(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rc.Close()

	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": a.Name})
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", disposition)
	io.Copy(w, rc) //nolint:errcheck
}

// handleDeleteArtifact deletes an artifact (projectAdmin required).
// DELETE /api/v1/artifacts/{aid}
func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	aid := r.PathValue("aid")
	if aid == "" {
		writeError(w, http.StatusBadRequest, "artifact ID required")
		return
	}

	svc := service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath)
	if err := svc.Delete(r.Context(), aid); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "artifact_id": aid})
}
