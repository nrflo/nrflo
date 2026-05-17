package spawner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// expandFindings replaces #{FINDINGS:AGENT:KEY} patterns with actual findings data.
// Patterns:
//   - #{FINDINGS:agent}           - All findings for agent
//   - #{FINDINGS:agent:key}       - Single specific key
//   - #{FINDINGS:agent:key1,key2} - Multiple specific keys
func (s *Spawner) expandFindings(template, projectID, ticketID, workflowName, wfiID string) (string, error) {
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

		findings, err := s.fetchFindings(projectID, ticketID, workflowName, agentType, wfiID, keys)
		if err != nil {
			lastErr = err
			return s.formatFindingsError(agentType)
		}

		return s.formatFindings(agentType, findings, keys)
	})

	return result, lastErr
}

// fetchFindings retrieves findings from the database using the FindingsService
func (s *Spawner) fetchFindings(projectID, ticketID, workflowName, agentType, wfiID string, keys []string) (interface{}, error) {
	pool := s.pool()
	if pool == nil {
		return nil, fmt.Errorf("failed to get database pool")
	}

	findingsService := service.NewFindingsService(pool, s.config.Clock)

	req := &types.FindingsGetRequest{
		AgentType:  agentType,
		Keys:       keys,
		InstanceID: wfiID,
	}

	return findingsService.Get(req)
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

// expandLayerFindings replaces #{LAYER_FINDINGS:N} and #{PRIOR_LAYER_FINDINGS} patterns.
// #{PRIOR_LAYER_FINDINGS} expands to layer currentLayer-1 (or "_No prior layer_" when currentLayer==0).
// #{LAYER_FINDINGS:N} expands to a flat sibling roster for layer N sorted by agent_type.
func (s *Spawner) expandLayerFindings(template string, currentLayer int, projectID, wfiID string) (string, error) {
	if !layerFindingsPattern.MatchString(template) {
		return template, nil
	}

	pool := s.pool()
	if pool == nil {
		return template, fmt.Errorf("failed to get database pool")
	}

	findingsSvc := service.NewFindingsService(pool, s.config.Clock)

	var lastErr error
	result := layerFindingsPattern.ReplaceAllStringFunc(template, func(match string) string {
		if match == "#{PRIOR_LAYER_FINDINGS}" {
			if currentLayer == 0 {
				return "_No prior layer_"
			}
			target := currentLayer - 1
			return s.fetchAndFormatLayerFindings(findingsSvc, target, wfiID, &lastErr)
		}
		// #{LAYER_FINDINGS:N}
		sub := layerFindingsPattern.FindStringSubmatch(match)
		if len(sub) < 2 || sub[1] == "" {
			return match
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return match
		}
		return s.fetchAndFormatLayerFindings(findingsSvc, n, wfiID, &lastErr)
	})

	return result, lastErr
}

// fetchAndFormatLayerFindings fetches layer findings from the service and formats them.
func (s *Spawner) fetchAndFormatLayerFindings(svc *service.FindingsService, layer int, wfiID string, lastErr *error) string {
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		*lastErr = err
		return fmt.Sprintf("_Error fetching layer %d findings_", layer)
	}
	layerMap, ok := result.(map[string]interface{})
	if !ok || len(layerMap) == 0 {
		return fmt.Sprintf("_No findings for layer %d_", layer)
	}
	return s.formatLayerFindings(layerMap)
}

// formatLayerFindings renders a flat agent_type-keyed roster sorted by agent_type.
// Each agent_type gets a header line; its findings are indented two spaces.
// Agents with nil or empty findings get "  _No findings_".
func (s *Spawner) formatLayerFindings(layerMap map[string]interface{}) string {
	var agentTypes []string
	for k := range layerMap {
		agentTypes = append(agentTypes, k)
	}
	sort.Strings(agentTypes)

	var lines []string
	for _, agentType := range agentTypes {
		lines = append(lines, agentType+":")
		val := layerMap[agentType]
		if val == nil {
			lines = append(lines, "  _No findings_")
			continue
		}
		agentFindings, ok := val.(map[string]interface{})
		if !ok || len(agentFindings) == 0 {
			lines = append(lines, "  _No findings_")
			continue
		}
		var keys []string
		for k := range agentFindings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, "  "+k+":"+s.formatValue(agentFindings[k], "  "))
		}
	}
	return strings.Join(lines, "\n")
}

// fetchPreviousDataAndReason retrieves to_resume data and result_reason from the most
// recent continued session for the same agent type, model, and phase.
// instanceID is optional — when set, used directly instead of DB lookup.
func (s *Spawner) fetchPreviousDataAndReason(projectID, ticketID, workflowName, agentType, modelID, phase, instanceID string) (data string, resultReason string) {
	if phase == "" {
		return "", ""
	}

	pool := s.pool()
	if pool == nil {
		return "", ""
	}

	wfiID := instanceID
	var err error
	if wfiID == "" {
		if ticketID == "" {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project' AND status = 'active'
				ORDER BY created_at DESC LIMIT 1`,
				projectID, workflowName).Scan(&wfiID)
		} else {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
				projectID, ticketID, workflowName).Scan(&wfiID)
		}
		if err != nil {
			return "", ""
		}
	}

	var sessionID string
	var reasonStr sql.NullString
	err = pool.QueryRow(`
		SELECT id, result_reason FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND phase = ? AND status = 'continued'
		ORDER BY ended_at DESC LIMIT 1`,
		wfiID, agentType, modelID, phase).Scan(&sessionID, &reasonStr)
	if err != nil {
		return "", ""
	}

	reason := ""
	if reasonStr.Valid {
		reason = reasonStr.String
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	rawFindings, err := findingRepo.GetOwn("session", sessionID)
	if err != nil || len(rawFindings) == 0 {
		return "", reason
	}

	rawVal, ok := rawFindings["to_resume"]
	if !ok {
		return "", reason
	}
	var str string
	if json.Unmarshal(rawVal, &str) != nil || str == "" {
		return "", reason
	}
	return str, reason
}
