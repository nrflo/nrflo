package spawner

import (
	"fmt"
	"strings"

	"be/internal/service"
	"be/internal/types"
)

// expandProjectFindings replaces #{PROJECT_FINDINGS:key} patterns with values
// from the project_findings table.
// Patterns:
//   - #{PROJECT_FINDINGS:key}       - Single key
//   - #{PROJECT_FINDINGS:k1,k2}     - Multiple keys (comma-separated)
func (s *Spawner) expandProjectFindings(template, projectID string) (string, error) {
	re := projectFindingsPattern

	var lastErr error
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		rawKeys := parts[1]
		keys := strings.Split(rawKeys, ",")
		for i := range keys {
			keys[i] = strings.TrimSpace(keys[i])
		}

		pool := s.pool()
		if pool == nil {
			lastErr = fmt.Errorf("failed to get database pool")
			return match
		}

		pfService := service.NewProjectFindingsService(pool, s.config.Clock)

		if len(keys) == 1 {
			val, err := pfService.Get(projectID, &types.ProjectFindingsGetRequest{Key: keys[0]})
			if err != nil {
				return fmt.Sprintf("_No project finding for key '%s'_", keys[0])
			}
			return strings.TrimPrefix(s.formatValue(val, ""), " ")
		}

		// Multiple keys
		result, err := pfService.Get(projectID, &types.ProjectFindingsGetRequest{Keys: keys})
		if err != nil {
			// All keys missing
			var placeholders []string
			for _, k := range keys {
				placeholders = append(placeholders, fmt.Sprintf("_No project finding for key '%s'_", k))
			}
			return strings.Join(placeholders, "\n")
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return strings.TrimPrefix(s.formatValue(result, ""), " ")
		}

		// Build output with found values and placeholders for missing keys
		findings := make(map[string]interface{})
		for _, k := range keys {
			if v, exists := resultMap[k]; exists {
				findings[k] = v
			}
		}

		var lines []string
		for _, k := range keys {
			if v, exists := findings[k]; exists {
				lines = append(lines, k+":"+s.formatValue(v, ""))
			} else {
				lines = append(lines, fmt.Sprintf("_No project finding for key '%s'_", k))
			}
		}
		return strings.Join(lines, "\n")
	})

	return result, lastErr
}
