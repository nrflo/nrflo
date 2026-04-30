package api

import (
	"net/http"
	"strconv"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func (s *Server) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	tasks, err := svc.List(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	var req types.ScheduledTaskCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	task, err := svc.Create(projectID, &req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventScheduleCreated, projectID, "", "", map[string]interface{}{"task_id": task.ID}))
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleGetScheduledTask(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	task, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req types.ScheduledTaskUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	task, err := svc.Update(id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventScheduleUpdated, projectID, "", "", map[string]interface{}{"task_id": id}))
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	if err := svc.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventScheduleDeleted, projectID, "", "", map[string]interface{}{"task_id": id}))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListScheduleRuns(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	taskID := r.PathValue("id")
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	svc := service.NewScheduledTaskService(s.pool, s.clock, s.scheduler)
	runs, err := svc.ListRuns(taskID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleRunScheduledTaskNow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	taskID := r.PathValue("id")
	if s.scheduler == nil {
		writeError(w, http.StatusInternalServerError, "scheduler not available")
		return
	}
	run, err := s.scheduler.RunNow(taskID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}
