package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"be/internal/repo"
	"be/internal/types"
)

// subLaneSpec is one worker/child session pulled from a delegations or
// consults row, before its agent_sessions row is known.
type subLaneSpec struct {
	sessionID       string
	callerSessionID string
	kind            string // "delegate" | "consult"
	delegationID    string
	consultID       string
	depth           int
}

// subLaneSessionRow is the agent_sessions data needed to render one sub-lane.
type subLaneSessionRow struct {
	phase, nodeID, agentType, modelID, status, result string
	segment                                           types.TraceSegment
	nudgeCount, stopBlockCount                        int
	timeBuckets                                       string
}

// loadTraceSubLanes reads the durable delegations/consults rows for wfiID and
// loads their worker/child agent_sessions rows by explicit session id — this
// sidesteps transientAgentTypeExclusion (delegate workers and consult
// children are underscore-node sessions hidden from the main trace lanes)
// rather than loosening it. Delegations are processed depth ASC so a nested
// T1→T2 worker parents onto its T1 sub-lane instead of falling back to root.
// Returns the built sub-lanes plus their session→lane entries for merging
// into the caller's sessionToLane index.
func (s *WorkflowService) loadTraceSubLanes(wfiID string, lanes []types.TraceLane, sessionToLane map[string]string) ([]types.TraceLane, map[string]string) {
	parentLane := make(map[string]string, len(sessionToLane))
	for sid, laneID := range sessionToLane {
		parentLane[sid] = laneID
	}
	laneLayer := make(map[string]int, len(lanes))
	for _, l := range lanes {
		laneLayer[l.LaneID] = l.Layer
	}

	specs := s.loadSubLaneSpecs(wfiID)
	if len(specs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(specs))
	for _, sp := range specs {
		ids = append(ids, sp.sessionID)
	}
	rows := s.querySubLaneSessions(ids)

	subLanes := make([]types.TraceLane, 0, len(specs))
	newSessionToLane := make(map[string]string, len(specs))
	for _, sp := range specs {
		row, ok := rows[sp.sessionID]
		if !ok {
			continue
		}
		layer := -1
		parentID := parentLane[sp.callerSessionID]
		if l, ok := laneLayer[parentID]; ok {
			layer = l
		}
		lane := types.TraceLane{
			LaneID:         sp.sessionID,
			Phase:          row.phase,
			NodeID:         row.nodeID,
			Layer:          layer,
			AgentType:      row.agentType,
			ModelID:        row.modelID,
			Status:         row.status,
			Result:         row.result,
			Segments:       []types.TraceSegment{row.segment},
			Markers:        []types.TraceMarker{},
			ParentLaneID:   parentID,
			Kind:           sp.kind,
			DelegationID:   sp.delegationID,
			ConsultID:      sp.consultID,
			Depth:          sp.depth,
			NudgeCount:     row.nudgeCount,
			StopBlockCount: row.stopBlockCount,
		}
		if row.timeBuckets != "" {
			addTimeBuckets(&lane, row.timeBuckets)
		}
		subLanes = append(subLanes, lane)
		// Nested delegate workers use their own sub-lane as caller context
		// for a deeper delegation processed later (depth ASC ordering).
		parentLane[sp.sessionID] = sp.sessionID
		laneLayer[sp.sessionID] = layer
		newSessionToLane[sp.sessionID] = sp.sessionID
	}

	return subLanes, newSessionToLane
}

// loadSubLaneSpecs reads delegations (depth ASC) and consults for wfiID and
// flattens their worker/child session ids into specs, skipping unspawned
// fanout slots (empty worker_session_ids entries).
func (s *WorkflowService) loadSubLaneSpecs(wfiID string) []subLaneSpec {
	var specs []subLaneSpec

	delRows, err := s.pool.Query(
		`SELECT id, caller_session_id, depth, worker_session_ids FROM delegations
		 WHERE workflow_instance_id = ? ORDER BY depth ASC, created_at ASC`, wfiID)
	if err == nil {
		for delRows.Next() {
			var id, callerSID, workerIDsJSON string
			var depth int
			if delRows.Scan(&id, &callerSID, &depth, &workerIDsJSON) != nil {
				continue
			}
			var workerIDs []string
			if json.Unmarshal([]byte(workerIDsJSON), &workerIDs) != nil {
				continue
			}
			for _, wsid := range workerIDs {
				if wsid == "" {
					continue
				}
				specs = append(specs, subLaneSpec{
					sessionID: wsid, callerSessionID: callerSID, kind: "delegate", delegationID: id, depth: depth,
				})
			}
		}
		delRows.Close()
	}

	consults, err := repo.NewConsultRepo(s.pool, s.clock).ListByInstance(wfiID)
	if err == nil {
		for _, c := range consults {
			if c.ChildSessionID == "" {
				continue
			}
			specs = append(specs, subLaneSpec{
				sessionID: c.ChildSessionID, callerSessionID: c.CallerSessionID, kind: "consult", consultID: c.ID,
			})
		}
	}

	return specs
}

// querySubLaneSessions loads agent_sessions rows by explicit id list — the
// mechanism that sidesteps transientAgentTypeExclusion for delegate-worker
// and consult-child sessions without loosening it.
func (s *WorkflowService) querySubLaneSessions(ids []string) map[string]subLaneSessionRow {
	out := make(map[string]subLaneSessionRow, len(ids))
	if len(ids) == 0 {
		return out
	}
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.pool.Query(fmt.Sprintf(`
		SELECT id, phase, node_id, agent_type, model_id, status, result, started_at, ended_at,
		       nudge_count, stop_block_count, time_buckets_json
		FROM agent_sessions WHERE id IN (%s)`, sqlPlaceholders(len(ids))), args...)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var phase, nodeID, agentType, modelID, status, result, startedAt, endedAt, timeBucketsJSON sql.NullString
		var nudgeCount, stopBlockCount sql.NullInt64
		if rows.Scan(&id, &phase, &nodeID, &agentType, &modelID, &status, &result, &startedAt, &endedAt,
			&nudgeCount, &stopBlockCount, &timeBucketsJSON) != nil {
			continue
		}
		seg := types.TraceSegment{SessionID: id, Status: status.String, Result: result.String}
		if startedAt.Valid {
			v := startedAt.String
			seg.StartedAt = &v
		}
		if endedAt.Valid {
			v := endedAt.String
			seg.EndedAt = &v
		}
		out[id] = subLaneSessionRow{
			phase: phase.String, nodeID: nodeID.String, agentType: agentType.String, modelID: modelID.String,
			status: status.String, result: result.String, segment: seg,
			nudgeCount: int(nudgeCount.Int64), stopBlockCount: int(stopBlockCount.Int64),
			timeBuckets: timeBucketsJSON.String,
		}
	}
	return out
}
