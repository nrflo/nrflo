package service

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"be/internal/types"
)

const (
	traceMarkerLimitDefault = 2000
	traceMarkerLimitMax     = 5000
	traceLabelMax           = 200
)

// traceMarkerCategories is the accepted set for the categories filter:
// agent_messages categories plus "finding" (findings_history markers).
var traceMarkerCategories = map[string]bool{
	"text": true, "tool": true, "subagent": true, "skill": true,
	"user_input": true, "error": true, "result": true, "validation": true,
	"thinking": true, "finding": true,
}

// traceDefaultCategories excludes high-volume, low-signal rows (text,
// thinking, result, validation) from the default timeline.
var traceDefaultCategories = []string{"tool", "subagent", "skill", "user_input", "error", "finding"}

// TraceOptions controls marker extraction for BuildTrace.
type TraceOptions struct {
	Categories  []string
	MarkerLimit int
}

// ParseTraceOptions validates raw query-param values into TraceOptions;
// empty params select the defaults.
func ParseTraceOptions(categoriesParam, markerLimitParam string) (TraceOptions, error) {
	opts := TraceOptions{Categories: traceDefaultCategories, MarkerLimit: traceMarkerLimitDefault}
	if categoriesParam != "" {
		var cats []string
		for _, c := range strings.Split(categoriesParam, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if !traceMarkerCategories[c] {
				return opts, fmt.Errorf("unknown category '%s'", c)
			}
			cats = append(cats, c)
		}
		if len(cats) == 0 {
			return opts, fmt.Errorf("categories must name at least one category")
		}
		opts.Categories = cats
	}
	if markerLimitParam != "" {
		n, err := strconv.Atoi(markerLimitParam)
		if err != nil || n < 1 || n > traceMarkerLimitMax {
			return opts, fmt.Errorf("marker_limit must be an integer between 1 and %d", traceMarkerLimitMax)
		}
		opts.MarkerLimit = n
	}
	return opts, nil
}

// attachTraceMarkers extracts point events (agent_messages rows + finding
// writes), attaches them to their lanes, and returns the markers that cannot
// be attributed to a session (instance-scope finding writes by unknown
// actors) plus whether the global cap truncated the set (earliest-first).
func (s *WorkflowService) attachTraceMarkers(wfiID string, lanes []types.TraceLane, sessionToLane map[string]string, opts TraceOptions) ([]types.TraceMarker, bool) {
	if len(opts.Categories) == 0 {
		opts.Categories = traceDefaultCategories
	}
	if opts.MarkerLimit <= 0 {
		opts.MarkerLimit = traceMarkerLimitDefault
	}

	var msgCats []string
	includeFindings := false
	for _, c := range opts.Categories {
		if c == "finding" {
			includeFindings = true
			continue
		}
		msgCats = append(msgCats, c)
	}

	sessionIDs := make([]string, 0, len(sessionToLane))
	for id := range sessionToLane {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)

	markers := s.queryMessageMarkers(sessionIDs, msgCats, opts.MarkerLimit+1)
	if includeFindings {
		markers = append(markers, s.queryFindingMarkers(wfiID, sessionIDs, opts.MarkerLimit+1)...)
	}
	sortTraceMarkers(markers)
	truncated := false
	if len(markers) > opts.MarkerLimit {
		markers = markers[:opts.MarkerLimit]
		truncated = true
	}

	laneIdx := make(map[string]int, len(lanes))
	for i := range lanes {
		laneIdx[lanes[i].LaneID] = i
	}
	rootMarkers := []types.TraceMarker{}
	for _, m := range markers {
		if laneID, ok := sessionToLane[m.SessionID]; ok {
			i := laneIdx[laneID]
			lanes[i].Markers = append(lanes[i].Markers, m)
		} else {
			rootMarkers = append(rootMarkers, m)
		}
	}
	return rootMarkers, truncated
}

// sortTraceMarkers orders markers by parsed timestamp (earliest first), with
// session id + label as deterministic tiebreaks for same-instant rows.
func sortTraceMarkers(markers []types.TraceMarker) {
	keys := make([]time.Time, len(markers))
	for i, m := range markers {
		keys[i], _ = parseTraceTime(m.At)
	}
	sort.SliceStable(markers, func(i, j int) bool {
		if !keys[i].Equal(keys[j]) {
			return keys[i].Before(keys[j])
		}
		if markers[i].SessionID != markers[j].SessionID {
			return markers[i].SessionID < markers[j].SessionID
		}
		return markers[i].Label < markers[j].Label
	})
}

// queryMessageMarkers pulls timeline rows from agent_messages across all of
// the instance's sessions. Labels are truncated in SQL; payload is never read.
func (s *WorkflowService) queryMessageMarkers(sessionIDs, categories []string, limit int) []types.TraceMarker {
	if len(sessionIDs) == 0 || len(categories) == 0 {
		return nil
	}
	args := make([]interface{}, 0, len(sessionIDs)+len(categories)+1)
	for _, id := range sessionIDs {
		args = append(args, id)
	}
	for _, c := range categories {
		args = append(args, c)
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT session_id, category, substr(content, 1, %d), created_at,
		       json_extract(payload, '$.ended_at')
		FROM agent_messages
		WHERE session_id IN (%s) AND category IN (%s)
		ORDER BY created_at, session_id, seq
		LIMIT ?`, traceLabelMax, sqlPlaceholders(len(sessionIDs)), sqlPlaceholders(len(categories)))
	rows, err := s.pool.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var markers []types.TraceMarker
	for rows.Next() {
		var m types.TraceMarker
		var endedAt sql.NullString
		rows.Scan(&m.SessionID, &m.Type, &m.Label, &m.At, &endedAt)
		if endedAt.Valid {
			v := endedAt.String
			m.EndedAt = &v
		}
		markers = append(markers, m)
	}
	return markers
}

// queryFindingMarkers pulls finding mutations from findings_history: session
// scope rows attach to the writing session; instance-scope rows attach via
// actor_id when it names a session, otherwise they surface as root markers.
func (s *WorkflowService) queryFindingMarkers(wfiID string, sessionIDs []string, limit int) []types.TraceMarker {
	var markers []types.TraceMarker
	if len(sessionIDs) > 0 {
		args := make([]interface{}, 0, len(sessionIDs)+1)
		for _, id := range sessionIDs {
			args = append(args, id)
		}
		args = append(args, limit)
		rows, err := s.pool.Query(fmt.Sprintf(`
			SELECT scope_id, key, operation, created_at
			FROM findings_history
			WHERE scope = 'session' AND scope_id IN (%s)
			ORDER BY created_at LIMIT ?`, sqlPlaceholders(len(sessionIDs))), args...)
		if err == nil {
			for rows.Next() {
				var sessionID, key, op, createdAt string
				rows.Scan(&sessionID, &key, &op, &createdAt)
				markers = append(markers, types.TraceMarker{
					Type: "finding", At: createdAt, SessionID: sessionID, Label: op + " " + key,
				})
			}
			rows.Close()
		}
	}

	rows, err := s.pool.Query(`
		SELECT key, operation, COALESCE(actor_id, ''), created_at
		FROM findings_history
		WHERE scope = 'workflow_instance' AND scope_id = ?
		ORDER BY created_at LIMIT ?`, wfiID, limit)
	if err != nil {
		return markers
	}
	defer rows.Close()
	for rows.Next() {
		var key, op, actorID, createdAt string
		rows.Scan(&key, &op, &actorID, &createdAt)
		markers = append(markers, types.TraceMarker{
			Type: "finding", At: createdAt, SessionID: actorID, Label: op + " " + key,
		})
	}
	return markers
}

// sqlPlaceholders returns "?,?,…" with n placeholders.
func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
