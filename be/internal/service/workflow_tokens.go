package service

// loadModelContextLengths queries cli_models and returns a map of model ID to context_length.
// On error, returns an empty map so callers fall back to the 200000 default.
func (s *WorkflowService) loadModelContextLengths() map[string]int64 {
	rows, err := s.pool.Query(`SELECT id, context_length FROM cli_models`)
	if err != nil {
		return map[string]int64{}
	}
	defer rows.Close()
	m := make(map[string]int64)
	for rows.Next() {
		var id string
		var ctxLen int64
		if err := rows.Scan(&id, &ctxLen); err != nil {
			continue
		}
		m[id] = ctxLen
	}
	return m
}
