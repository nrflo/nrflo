package spawner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
)

// expandFindings replaces #{FINDINGS:AGENT:KEY} patterns with actual findings data.
// Patterns:
//   - #{FINDINGS:agent}           - All findings for agent
//   - #{FINDINGS:agent:key}       - Single specific key
//   - #{FINDINGS:agent:key1,key2} - Multiple specific keys
func (s *Spawner) expandFindings(template, projectID, ticketID, workflowName string) (string, error) {
	// Pattern: #{FINDINGS:agent_type} or #{FINDINGS:agent_type:key(s)}
	re := findingsPattern

	var lastErr error
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		agentType := parts[1]
		var keys []string
		if len(parts) >= 3 && parts[2] != "" {
			keys = strings.Split(parts[2], ",")
			for i := range keys {
				keys[i] = strings.TrimSpace(keys[i])
			}
		}

		findings, err := s.fetchFindings(projectID, ticketID, workflowName, agentType, keys)
		if err != nil {
			lastErr = err
			return s.formatFindingsError(agentType)
		}

		return s.formatFindings(agentType, findings, keys)
	})

	return result, lastErr
}

// fetchFindings retrieves findings from the database using the FindingsService
func (s *Spawner) fetchFindings(projectID, ticketID, workflowName, agentType string, keys []string) (interface{}, error) {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	findingsService := service.NewFindingsService(pool)

	req := &types.FindingsGetRequest{
		Workflow:  workflowName,
		AgentType: agentType,
		Keys:      keys,
	}

	return findingsService.Get(projectID, ticketID, req)
}

// formatFindings converts findings to human-readable text (YAML-like format)
func (s *Spawner) formatFindings(agentType string, findings interface{}, keys []string) string {
	if findings == nil {
		return s.formatFindingsError(agentType)
	}

	findingsMap, ok := findings.(map[string]interface{})
	if !ok {
		return s.formatValue(findings, "")
	}

	if len(findingsMap) == 0 {
		return s.formatFindingsError(agentType)
	}

	isParallel := false
	for k := range findingsMap {
		if strings.Contains(k, ":") {
			isParallel = true
			break
		}
	}

	if isParallel {
		return s.formatParallelFindings(agentType, findingsMap, keys)
	}

	return s.formatSingleAgentFindings(findingsMap)
}

// formatParallelFindings formats findings from multiple parallel agents
func (s *Spawner) formatParallelFindings(agentType string, findings map[string]interface{}, keys []string) string {
	var lines []string

	var modelKeys []string
	for k := range findings {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	for _, modelKey := range modelKeys {
		agentKey := agentType + ":" + modelKey
		v := findings[modelKey]

		if len(keys) == 1 {
			lines = append(lines, fmt.Sprintf("- %s: %s", agentKey, s.formatValue(v, "")))
		} else {
			lines = append(lines, fmt.Sprintf("- %s:", agentKey))
			if agentFindings, ok := v.(map[string]interface{}); ok {
				var sortedKeys []string
				for k := range agentFindings {
					sortedKeys = append(sortedKeys, k)
				}
				sort.Strings(sortedKeys)
				for _, k := range sortedKeys {
					val := agentFindings[k]
					lines = append(lines, "  "+k+":"+s.formatValue(val, "  "))
				}
			} else {
				lines = append(lines, "  "+s.formatValue(v, "  "))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// formatSingleAgentFindings formats findings from a single agent as "key: value" lines
func (s *Spawner) formatSingleAgentFindings(findings map[string]interface{}) string {
	var lines []string

	var keys []string
	for k := range findings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := findings[key]
		lines = append(lines, key+":"+s.formatValue(val, ""))
	}
	return strings.Join(lines, "\n")
}

// formatValue converts any value to YAML-like text (never JSON)
func (s *Spawner) formatValue(v interface{}, indent string) string {
	switch val := v.(type) {
	case string:
		return " " + val
	case []interface{}:
		var lines []string
		for _, item := range val {
			itemStr := s.formatValue(item, indent+"  ")
			itemStr = strings.TrimPrefix(itemStr, " ")
			lines = append(lines, indent+"  - "+itemStr)
		}
		return "\n" + strings.Join(lines, "\n")
	case map[string]interface{}:
		var lines []string
		var keys []string
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, indent+"  "+k+":"+s.formatValue(val[k], indent+"  "))
		}
		return "\n" + strings.Join(lines, "\n")
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf(" %d", int(val))
		}
		return fmt.Sprintf(" %v", val)
	case bool:
		return fmt.Sprintf(" %v", val)
	case nil:
		return " null"
	default:
		return fmt.Sprintf(" %v", val)
	}
}

// formatFindingsError returns a placeholder for missing findings
func (s *Spawner) formatFindingsError(agentType string) string {
	return fmt.Sprintf("_No findings yet available from %s_", agentType)
}

// fetchPreviousData retrieves findings from the most recent continued session
// for the same agent type, model, and phase. Returns empty string if none found.
func (s *Spawner) fetchPreviousData(projectID, ticketID, workflowName, agentType, modelID, phase string) string {
	if phase == "" {
		return ""
	}

	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return ""
	}
	defer database.Close()

	var wfiID string
	if ticketID == "" {
		// Project-scoped workflow
		err = database.QueryRow(`
			SELECT id FROM workflow_instances
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project'`,
			projectID, workflowName).Scan(&wfiID)
	} else {
		err = database.QueryRow(`
			SELECT id FROM workflow_instances
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
			projectID, ticketID, workflowName).Scan(&wfiID)
	}
	if err != nil {
		return ""
	}

	var findingsStr string
	err = database.QueryRow(`
		SELECT findings FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND phase = ? AND status = 'continued'
		  AND findings IS NOT NULL AND findings != ''
		ORDER BY ended_at DESC LIMIT 1`,
		wfiID, agentType, modelID, phase).Scan(&findingsStr)
	if err != nil || findingsStr == "" {
		return ""
	}

	var findings map[string]interface{}
	if json.Unmarshal([]byte(findingsStr), &findings) != nil || len(findings) == 0 {
		return ""
	}

	return s.formatSingleAgentFindings(findings)
}
