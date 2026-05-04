package api

import (
	"net/http"

	"be/internal/service"
)

func (s *Server) handleInsightsSummary(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "7d"
	}
	since, err := service.ParseRange(rangeStr, s.clock)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc := service.NewInsightsService(s.pool, s.clock)
	summary, err := svc.Summary(projectID, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleInsightsEditRate(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "7d"
	}
	since, err := service.ParseRange(rangeStr, s.clock)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc := service.NewInsightsService(s.pool, s.clock)
	rows, err := svc.EditRate(projectID, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []*service.EditRateResult{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleInsightsThroughput(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "7d"
	}
	since, err := service.ParseRange(rangeStr, s.clock)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bucketStr := r.URL.Query().Get("bucket")
	if bucketStr == "" {
		if rangeStr == "30d" {
			bucketStr = "6h"
		} else {
			bucketStr = "1h"
		}
	}
	bucket, err := service.ParseBucket(bucketStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc := service.NewInsightsService(s.pool, s.clock)
	points, err := svc.Throughput(projectID, since, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if points == nil {
		points = nil // keep as null in JSON for empty series
	}
	writeJSON(w, http.StatusOK, points)
}
