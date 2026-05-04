package api

import (
	"net/http"
	"strconv"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	status := r.URL.Query().Get("status")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, _ := strconv.Atoi(v); n >= 0 {
			offset = n
		}
	}
	rr := repo.NewReviewRepo(s.pool, s.clock)
	items, err := rr.List(projectID, status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*model.ReviewItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

type reviewCreateRequest struct {
	ToolName  string  `json:"tool_name"`
	SessionID *string `json:"session_id"`
	Input     string  `json:"input"`
	Output    *string `json:"output"`
	Draft     *string `json:"draft"`
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	var req reviewCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ToolName == "" {
		writeError(w, http.StatusBadRequest, "tool_name is required")
		return
	}
	if req.Input == "" {
		writeError(w, http.StatusBadRequest, "input is required")
		return
	}
	item := &model.ReviewItem{
		ProjectID: projectID,
		ToolName:  req.ToolName,
		SessionID: req.SessionID,
		Input:     req.Input,
		Output:    req.Output,
		Draft:     req.Draft,
	}
	rr := repo.NewReviewRepo(s.pool, s.clock)
	if err := rr.Insert(item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type reviewWithDiff struct {
	*model.ReviewItem
	Diff interface{} `json:"diff,omitempty"`
}

func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	rr := repo.NewReviewRepo(s.pool, s.clock)
	item, err := rr.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "review item not found")
		return
	}
	resp := reviewWithDiff{ReviewItem: item}
	if item.Draft != nil {
		resp.Diff = diffJSON(item.Input, *item.Draft)
	}
	writeJSON(w, http.StatusOK, resp)
}

type reviewDraftRequest struct {
	Draft string `json:"draft"`
}

func (s *Server) handlePatchReview(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req reviewDraftRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rr := repo.NewReviewRepo(s.pool, s.clock)
	if err := rr.UpdateDraft(id, projectID, req.Draft); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(&ws.Event{
		Type:      ws.EventReviewUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"review_item_id": id, "status": "pending"},
	})
	item, err := rr.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleApproveReview(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	rr := repo.NewReviewRepo(s.pool, s.clock)
	if err := rr.Approve(id, projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(&ws.Event{
		Type:      ws.EventReviewUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"review_item_id": id, "status": "approved"},
	})
	item, err := rr.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type reviewRejectRequest struct {
	Reason string `json:"reason"`
}

func (s *Server) handleRejectReview(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req reviewRejectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rr := repo.NewReviewRepo(s.pool, s.clock)
	if err := rr.Reject(id, projectID, req.Reason); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(&ws.Event{
		Type:      ws.EventReviewUpdated,
		ProjectID: projectID,
		Data:      map[string]interface{}{"review_item_id": id, "status": "rejected"},
	})
	item, err := rr.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}
