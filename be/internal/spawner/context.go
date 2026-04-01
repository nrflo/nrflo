package spawner

import (
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/ws"
)

// readContextLeftFromDB reads context_left from the database for all running processes
// and updates each proc.contextLeft in place.
func readContextLeftFromDB(pool *db.Pool, procs []*processInfo) {
	if pool == nil || len(procs) == 0 {
		return
	}

	// Build IN clause
	ids := make([]string, len(procs))
	args := make([]interface{}, len(procs))
	for i, p := range procs {
		ids[i] = "?"
		args[i] = p.sessionID
	}

	query := fmt.Sprintf(
		`SELECT id, context_left FROM agent_sessions WHERE id IN (%s)`,
		strings.Join(ids, ","))

	rows, err := pool.Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	// Build lookup
	contextMap := make(map[string]int)
	for rows.Next() {
		var id string
		var contextLeft int
		if rows.Scan(&id, &contextLeft) == nil {
			contextMap[id] = contextLeft
		}
	}

	// Update procs (only decrease — never overwrite a lower in-memory value with a higher DB value)
	for _, p := range procs {
		if cl, ok := contextMap[p.sessionID]; ok {
			if p.contextLeft == 0 || cl < p.contextLeft {
				p.contextLeft = cl
			}
		}
	}
}

// updateClaudeContext extracts usage from a Claude assistant or result event and updates context %.
func (s *Spawner) updateClaudeContext(proc *processInfo, data map[string]interface{}) {
	usage, _ := data["usage"].(map[string]interface{})
	if usage == nil {
		if msg, ok := data["message"].(map[string]interface{}); ok {
			usage, _ = msg["usage"].(map[string]interface{})
		}
	}
	if usage == nil {
		return
	}
	input, _ := usage["input_tokens"].(float64)
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	cacheCreate, _ := usage["cache_creation_input_tokens"].(float64)
	output, _ := usage["output_tokens"].(float64)
	totalUsed := int(input + cacheRead + cacheCreate + output)
	if totalUsed == 0 {
		return
	}
	maxCtx := proc.maxContext
	if maxCtx <= 0 {
		maxCtx = 200000
	}
	pctLeft := 100 - (totalUsed * 100 / maxCtx)
	if pctLeft < 0 {
		pctLeft = 0
	}
	if proc.contextLeft == 0 || pctLeft < proc.contextLeft {
		proc.contextLeft = pctLeft
		s.updateContextLeft(proc)
	}
}

// updateContextLeft persists context_left to the database and broadcasts a WS event.
// Called from updateClaudeContext when assistant/result events provide token usage.
func (s *Spawner) updateContextLeft(proc *processInfo) {
	pool := s.pool()
	if pool == nil {
		return
	}

	_, err := pool.Exec(
		`UPDATE agent_sessions SET context_left = ? WHERE id = ?`,
		proc.contextLeft, proc.sessionID)
	if err != nil {
		return
	}

	s.broadcast(ws.EventAgentContextUpdated, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"session_id":   proc.sessionID,
		"context_left": proc.contextLeft,
	})
}
