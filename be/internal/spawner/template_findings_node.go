package spawner

import (
	"context"
	"strings"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/types"
)

// expandNodeFindings replaces #{NODE_FINDINGS:node_id} and
// #{NODE_FINDINGS:node_id:key1,key2} patterns with findings attributed to a
// single execution node (read-time attribution via agent_sessions.node_id).
// An unknown node_id expands to "" with a logged warning (same convention as
// #{ARTIFACT:name}); a known node with no findings yet renders the standard
// missing-findings placeholder.
func (s *Spawner) expandNodeFindings(template, wfiID string) string {
	if !nodeFindingsPattern.MatchString(template) {
		return template
	}

	pool := s.pool()
	if pool == nil {
		return template
	}
	findingsSvc := service.NewFindingsService(pool, s.config.Clock)

	return nodeFindingsPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := nodeFindingsPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		nodeID := parts[1]
		var keys []string
		if len(parts) >= 3 && parts[2] != "" {
			keys = strings.Split(parts[2], ",")
			for i := range keys {
				keys[i] = strings.TrimSpace(keys[i])
			}
		}

		findings, err := findingsSvc.Get(&types.FindingsGetRequest{NodeID: nodeID, Keys: keys, InstanceID: wfiID})
		if err != nil {
			logger.Warn(context.Background(), "node findings expansion failed", "node_id", nodeID, "keys", keys, "error", err)
			return ""
		}

		return s.formatFindings(nodeID, findings, keys)
	})
}
