package service

import (
	"time"

	"be/internal/model"
	"be/internal/types"
)

// BuildTrace assembles the read-time span tree for one workflow instance:
// lanes (relaunch chains merged), layer bands, point-event markers, and child
// sub-workflow references. No state is written; everything derives from
// agent_sessions, agent_messages, findings_history, and workflow_instances.
func (s *WorkflowService) BuildTrace(iid string, opts TraceOptions) (*types.TraceResponse, error) {
	wi, err := s.wfiRepo.Get(iid)
	if err != nil {
		return nil, err
	}

	// Node→layer map from the current def; a deleted/renamed def degrades to
	// layer -1 lanes and no bands (same tolerance as buildV4State).
	nodeLayers := map[string]int{}
	var defPhases []PhaseDef
	if wf, defErr := s.GetWorkflowDef(wi.ProjectID, wi.WorkflowID); defErr == nil {
		defPhases = wf.Phases
		for _, p := range wf.Phases {
			nodeLayers[p.NodeID] = p.Layer
		}
	}

	lanes, sessionToLane, lifecycle := s.loadTraceLanes(iid, nodeLayers)
	subLanes, subSessionToLane := s.loadTraceSubLanes(iid, lanes, sessionToLane)
	for sid, laneID := range subSessionToLane {
		sessionToLane[sid] = laneID
	}
	s.attachTraceRestarts(iid, lanes)
	rootMarkers, truncated := s.attachTraceMarkers(iid, lanes, subLanes, sessionToLane, lifecycle, opts)

	resp := &types.TraceResponse{
		InstanceID:  wi.ID,
		ProjectID:   wi.ProjectID,
		TicketID:    wi.TicketID,
		Workflow:    wi.WorkflowID,
		Status:      string(wi.Status),
		StartedAt:   wi.CreatedAt.Format(time.RFC3339Nano),
		Layers:      buildTraceLayers(defPhases, lanes),
		Lanes:       lanes,
		SubLanes:    subLanes,
		Children:    s.loadTraceChildren(iid),
		RootMarkers: rootMarkers,
		Truncated:   truncated,
	}
	if isTerminalInstanceStatus(wi.Status) {
		ended := wi.UpdatedAt.Format(time.RFC3339Nano)
		resp.EndedAt = &ended
	}
	return resp, nil
}

func isTerminalInstanceStatus(st model.WorkflowInstanceStatus) bool {
	switch st {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceFailed, model.WorkflowInstanceProjectCompleted:
		return true
	}
	return false
}

// loadTraceChildren lists sub-workflow runs launched by this instance (one
// level deep; the UI navigates into a child's own trace for deeper nesting).
func (s *WorkflowService) loadTraceChildren(iid string) []types.TraceChildRun {
	children := []types.TraceChildRun{}
	instances, err := s.wfiRepo.ListByParentInstance(iid)
	if err != nil {
		return children
	}
	for _, wi := range instances {
		child := types.TraceChildRun{
			InstanceID: wi.ID,
			Workflow:   wi.WorkflowID,
			Status:     string(wi.Status),
			StartedAt:  wi.CreatedAt.Format(time.RFC3339Nano),
		}
		if isTerminalInstanceStatus(wi.Status) {
			ended := wi.UpdatedAt.Format(time.RFC3339Nano)
			child.EndedAt = &ended
		}
		if wi.ParentSession.Valid {
			child.ParentSessionID = wi.ParentSession.String
		}
		children = append(children, child)
	}
	return children
}

// attachTraceRestarts copies loadRestartDetails output onto lanes; both are
// keyed by the chain-root session id, so this is a direct lookup per lane.
func (s *WorkflowService) attachTraceRestarts(iid string, lanes []types.TraceLane) {
	details := s.loadRestartDetails(iid)
	for i := range lanes {
		for _, d := range details[lanes[i].LaneID] {
			lanes[i].Restarts = append(lanes[i].Restarts, types.TraceRestart{
				Reason:       d.Reason,
				DurationSec:  d.DurationSec,
				ContextLeft:  d.ContextLeft,
				MessageCount: d.MessageCount,
			})
		}
	}
}
