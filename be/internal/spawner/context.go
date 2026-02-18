package spawner

import (
	"fmt"
	"strings"

	"be/internal/db"
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

	// Update procs
	for _, p := range procs {
		if cl, ok := contextMap[p.sessionID]; ok {
			p.contextLeft = cl
		}
	}
}
