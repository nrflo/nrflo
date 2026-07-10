package service

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"be/internal/types"
)

// isOpenSessionStatus reports whether an agent session is still in flight.
func isOpenSessionStatus(status string) bool {
	return status == "running" || status == "user_interactive"
}

// parseTraceTime parses an agent_sessions/agent_messages timestamp.
// RFC3339Nano accepts plain RFC3339 too (fractional seconds are optional).
func parseTraceTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// loadTraceLanes queries agent_sessions for the instance and merges relaunch
// chains into single lanes keyed by COALESCE(ancestor_session_id, id) — the
// ancestor column always points at the chain root. Returns the sorted lanes
// plus a session→lane index used for marker attribution.
func (s *WorkflowService) loadTraceLanes(wfiID string, phaseLayers map[string]int) ([]types.TraceLane, map[string]string, []types.TraceMarker) {
	sessionToLane := make(map[string]string)
	lanes := []types.TraceLane{}
	var lifecycle []types.TraceMarker
	rows, err := s.pool.Query(`
		SELECT COALESCE(ancestor_session_id, id), id, phase, agent_type, model_id, status, result, started_at, ended_at,
		       result_reason, rate_limit_until_ts, rate_limit_retry_count, nudge_count, stop_block_count
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND `+transientAgentTypeExclusion+`
		ORDER BY started_at, created_at`, wfiID)
	if err != nil {
		return lanes, sessionToLane, lifecycle
	}
	defer rows.Close()

	laneIdx := make(map[string]int)
	for rows.Next() {
		var laneID, id string
		var phase, agentType, modelID, status, result, startedAt, endedAt, resultReason, rateLimitUntil sql.NullString
		var rateLimitRetries, nudgeCount, stopBlockCount sql.NullInt64
		rows.Scan(&laneID, &id, &phase, &agentType, &modelID, &status, &result, &startedAt, &endedAt,
			&resultReason, &rateLimitUntil, &rateLimitRetries, &nudgeCount, &stopBlockCount)

		seg := types.TraceSegment{SessionID: id, Status: status.String, Result: result.String}
		if startedAt.Valid {
			v := startedAt.String
			seg.StartedAt = &v
		}
		if endedAt.Valid {
			v := endedAt.String
			seg.EndedAt = &v
		}

		idx, ok := laneIdx[laneID]
		if !ok {
			layer := -1
			if l, found := phaseLayers[phase.String]; found {
				layer = l
			}
			lanes = append(lanes, types.TraceLane{
				LaneID:    laneID,
				Phase:     phase.String,
				Layer:     layer,
				AgentType: agentType.String,
				Segments:  []types.TraceSegment{},
				Markers:   []types.TraceMarker{},
			})
			idx = len(lanes) - 1
			laneIdx[laneID] = idx
		}
		lanes[idx].Segments = append(lanes[idx].Segments, seg)
		lanes[idx].Status = status.String
		lanes[idx].Result = result.String
		if modelID.Valid && modelID.String != "" {
			lanes[idx].ModelID = modelID.String
		}
		lanes[idx].NudgeCount += int(nudgeCount.Int64)
		lanes[idx].StopBlockCount += int(stopBlockCount.Int64)
		sessionToLane[id] = laneID

		lifecycle = append(lifecycle, sessionLifecycleMarkers(id, resultReason, endedAt, rateLimitUntil, rateLimitRetries)...)
	}

	sortTraceLanes(lanes)
	return lanes, sessionToLane, lifecycle
}

// sessionLifecycleMarkers derives orchestration point events from session
// columns: why a segment ended (result_reason at ended_at) and rate-limit
// waits (at rate_limit_until_ts). Counters without timestamps (nudges, stop
// blocks) surface as lane badges instead — they cannot sit on a time axis.
func sessionLifecycleMarkers(sessionID string, resultReason, endedAt, rateLimitUntil sql.NullString, rateLimitRetries sql.NullInt64) []types.TraceMarker {
	var markers []types.TraceMarker
	if resultReason.Valid && resultReason.String != "" && endedAt.Valid {
		markers = append(markers, types.TraceMarker{
			Type: "lifecycle", At: endedAt.String, SessionID: sessionID, Label: resultReason.String,
		})
	}
	if rateLimitUntil.Valid && rateLimitUntil.String != "" {
		label := "rate_limited"
		if rateLimitRetries.Int64 > 0 {
			label = fmt.Sprintf("rate_limited (retry %d)", rateLimitRetries.Int64)
		}
		markers = append(markers, types.TraceMarker{
			Type: "lifecycle", At: rateLimitUntil.String, SessionID: sessionID, Label: label,
		})
	}
	return markers
}

// sortTraceLanes orders lanes (layer asc, phase asc, first start asc, lane id
// asc); layer -1 (phase absent from the current def) sorts last.
func sortTraceLanes(lanes []types.TraceLane) {
	firstStart := func(l types.TraceLane) time.Time {
		for _, seg := range l.Segments {
			if seg.StartedAt != nil {
				if t, ok := parseTraceTime(*seg.StartedAt); ok {
					return t
				}
			}
		}
		return time.Time{}
	}
	effLayer := func(layer int) int {
		if layer < 0 {
			return math.MaxInt
		}
		return layer
	}
	sort.SliceStable(lanes, func(i, j int) bool {
		li, lj := effLayer(lanes[i].Layer), effLayer(lanes[j].Layer)
		if li != lj {
			return li < lj
		}
		if lanes[i].Phase != lanes[j].Phase {
			return lanes[i].Phase < lanes[j].Phase
		}
		si, sj := firstStart(lanes[i]), firstStart(lanes[j])
		if !si.Equal(sj) {
			return si.Before(sj)
		}
		return lanes[i].LaneID < lanes[j].LaneID
	})
}

// buildTraceLayers derives per-layer time bands from the def's phases and the
// assembled lanes. Every layer in the def is emitted, including pending ones
// with no sessions yet; a band stays open (nil ended_at) while any session in
// it is in flight.
func buildTraceLayers(defPhases []PhaseDef, lanes []types.TraceLane) []types.TraceLayer {
	type band struct {
		start, end       time.Time
		hasStart, hasEnd bool
		open             bool
	}
	phasesByLayer := map[int][]string{}
	var layerOrder []int
	for _, p := range defPhases {
		if _, seen := phasesByLayer[p.Layer]; !seen {
			layerOrder = append(layerOrder, p.Layer)
		}
		phasesByLayer[p.Layer] = append(phasesByLayer[p.Layer], p.ID)
	}
	sort.Ints(layerOrder)

	bands := map[int]*band{}
	for _, layer := range layerOrder {
		bands[layer] = &band{}
	}
	for _, lane := range lanes {
		b, ok := bands[lane.Layer]
		if !ok {
			continue
		}
		for _, seg := range lane.Segments {
			if seg.StartedAt != nil {
				if t, tok := parseTraceTime(*seg.StartedAt); tok {
					if !b.hasStart || t.Before(b.start) {
						b.start = t
						b.hasStart = true
					}
				}
			}
			if isOpenSessionStatus(seg.Status) {
				b.open = true
			} else if seg.EndedAt != nil {
				if t, tok := parseTraceTime(*seg.EndedAt); tok {
					if !b.hasEnd || t.After(b.end) {
						b.end = t
						b.hasEnd = true
					}
				}
			}
		}
	}

	layers := make([]types.TraceLayer, 0, len(layerOrder))
	for _, layer := range layerOrder {
		b := bands[layer]
		tl := types.TraceLayer{Layer: layer, Phases: phasesByLayer[layer]}
		if b.hasStart {
			v := b.start.Format(time.RFC3339Nano)
			tl.StartedAt = &v
		}
		if b.hasEnd && !b.open {
			v := b.end.Format(time.RFC3339Nano)
			tl.EndedAt = &v
		}
		layers = append(layers, tl)
	}
	return layers
}
