package api

import (
	"errors"
	"net/http"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// registerPlanRoutes registers the workflow plan lifecycle routes
// (draft/revise/approve/cancel a planner-authored manifest), called from
// registerRoutes next to the other workflow-instance routes.
func (s *Server) registerPlanRoutes(protected func(string, http.HandlerFunc)) {
	protected("GET /api/v1/workflow-instances/{iid}/plan", s.handleGetPlan)
	protected("GET /api/v1/workflow-instances/{iid}/plan/revisions", s.handleListPlanRevisions)
	protected("POST /api/v1/workflow-instances/{iid}/plan/revise", s.handleRevisePlan)
	protected("POST /api/v1/workflow-instances/{iid}/plan/approve", s.handleApprovePlan)
	protected("POST /api/v1/workflow-instances/{iid}/plan/cancel", s.handleCancelPlan)
}

// resolvePlanInstance loads the workflow instance for a plan route and, for
// write routes, enforces the __global__ admin-only guard. Returns nil (after
// writing the HTTP response) on any failure.
func (s *Server) resolvePlanInstance(w http.ResponseWriter, r *http.Request, iid string, isWrite bool) *planInstanceCtx {
	wfi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(iid)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow instance not found")
		return nil
	}
	if isWrite && denyNonAdminGlobalWrite(w, r, wfi.ProjectID) {
		return nil
	}
	return &planInstanceCtx{ProjectID: wfi.ProjectID, TicketID: wfi.TicketID, Workflow: wfi.WorkflowID}
}

type planInstanceCtx struct {
	ProjectID string
	TicketID  string
	Workflow  string
}

func (s *Server) planBroadcast(eventType string, ctx *planInstanceCtx, data map[string]interface{}) {
	if s.wsHub == nil || ctx == nil {
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(eventType, ctx.ProjectID, ctx.TicketID, ctx.Workflow, data))
}

// writePlanServiceError maps PlanService sentinel errors to their HTTP status.
func writePlanServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrStalePlanRevision):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrPlanNotDraft):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// handleGetPlan returns the plan head, latest manifest, and template library.
func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	if s.resolvePlanInstance(w, r, iid, false) == nil {
		return
	}
	svc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
	draft, err := svc.GetDraft(iid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

// handleListPlanRevisions returns every revision for a workflow instance's plan.
func (s *Server) handleListPlanRevisions(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	if s.resolvePlanInstance(w, r, iid, false) == nil {
		return
	}
	svc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
	revisions, err := svc.ListRevisions(iid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revisions": revisions})
}

// handleRevisePlan appends a new plan revision from either a caller-edited
// manifest or planner feedback/answers.
func (s *Server) handleRevisePlan(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	ctx := s.resolvePlanInstance(w, r, iid, true)
	if ctx == nil {
		return
	}
	var req types.PlanReviseRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
	rev, err := svc.Revise(r.Context(), iid, req)
	if err != nil {
		writePlanServiceError(w, err)
		return
	}
	eventType := ws.EventPlanRevised
	if rev.Revision == 1 {
		eventType = ws.EventPlanDrafted
	}
	s.planBroadcast(eventType, ctx, map[string]interface{}{
		"instance_id": iid,
		"revision":    rev.Revision,
		"author":      rev.Author,
	})
	writeJSON(w, http.StatusOK, rev)
}

// handleApprovePlan approves a plan at a specific (revision-pinned) revision.
func (s *Server) handleApprovePlan(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	ctx := s.resolvePlanInstance(w, r, iid, true)
	if ctx == nil {
		return
	}
	var req types.PlanApproveRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
	rev, err := svc.Approve(iid, req.Revision)
	if err != nil {
		writePlanServiceError(w, err)
		return
	}
	s.planBroadcast(ws.EventPlanApproved, ctx, map[string]interface{}{
		"instance_id": iid,
		"revision":    rev.Revision,
	})
	writeJSON(w, http.StatusOK, rev)
}

// handleCancelPlan cancels a draft plan.
func (s *Server) handleCancelPlan(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	ctx := s.resolvePlanInstance(w, r, iid, true)
	if ctx == nil {
		return
	}
	svc := service.NewPlanService(s.pool, s.clock, s.orchestrator)
	if err := svc.Cancel(iid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.planBroadcast(ws.EventPlanCancelled, ctx, map[string]interface{}{"instance_id": iid})
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
