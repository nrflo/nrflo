package service

type modelContextLengths struct {
	cli int64
	api int64
}

// loadModelContextLengths returns both mode-specific context windows by model ID.
// On error, returns an empty map so callers fall back to the 200000 default.
func (s *WorkflowService) loadModelContextLengths() map[string]modelContextLengths {
	rows, err := s.pool.Query(`SELECT id, cli_context, api_context FROM models`)
	if err != nil {
		return map[string]modelContextLengths{}
	}
	defer rows.Close()
	m := make(map[string]modelContextLengths)
	for rows.Next() {
		var id string
		var lengths modelContextLengths
		if err := rows.Scan(&id, &lengths.cli, &lengths.api); err != nil {
			continue
		}
		m[id] = lengths
	}
	return m
}
