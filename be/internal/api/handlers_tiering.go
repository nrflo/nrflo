package api

import (
	"net/http"

	"be/internal/service"
	"be/internal/types"
)

// handleTieringReport handles GET /api/v1/admin/tiering-report: a dry-run,
// cross-project listing of every tiered agent_definitions row's current vs
// recommended model/effort plus an estimated monthly cost delta. Admin-only,
// global (no X-Project scope) — mirrors handleListUsers.
func (s *Server) handleTieringReport(w http.ResponseWriter, _ *http.Request) {
	svc := service.NewTieringService(s.pool, s.clock, service.NewModelService(s.pool, s.clock))
	report, err := svc.BuildReport()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// handleApplyTiering handles POST /api/v1/admin/tiering-apply: applies the
// re-tier map to one or more projects' confirmed defs, each behind its own
// explicit per-project confirmation payload (explicit def keys, or
// confirm_all).
func (s *Server) handleApplyTiering(w http.ResponseWriter, r *http.Request) {
	var req types.TieringApplyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Confirmations) == 0 {
		writeError(w, http.StatusBadRequest, "confirmations is required")
		return
	}

	svc := service.NewTieringService(s.pool, s.clock, service.NewModelService(s.pool, s.clock))
	result := &types.TieringApplyResult{}
	for _, confirmation := range req.Confirmations {
		projectResult, err := svc.ApplyForProject(confirmation)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result.Applied = append(result.Applied, projectResult.Applied...)
		result.Skipped = append(result.Skipped, projectResult.Skipped...)
	}
	writeJSON(w, http.StatusOK, result)
}
