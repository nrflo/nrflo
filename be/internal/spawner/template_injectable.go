package spawner

import (
	"context"
	"regexp"
	"strings"

	"be/internal/logger"
)

var injectablePlaceholderRe = regexp.MustCompile(`\$\{[^}]+\}`)

// expandInjectable loads an injectable template from default_templates and expands vars.
// Returns "" with a warning log if the template is not found.
func (s *Spawner) expandInjectable(id string, vars map[string]string) string {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for injectable template", "id", id)
		return ""
	}

	var body string
	err := pool.QueryRow(`SELECT template FROM default_templates WHERE id = ? AND type = 'injectable'`, id).Scan(&body)
	if err != nil {
		logger.Warn(context.Background(), "injectable template not found", "id", id, "error", err)
		return ""
	}

	for k, v := range vars {
		body = strings.ReplaceAll(body, "${"+k+"}", v)
	}

	body = injectablePlaceholderRe.ReplaceAllString(body, "")

	return body
}

// isContinuationReason returns true for result_reason values that indicate the agent
// was interrupted without saving state (stall, fail, timeout restarts).
func isContinuationReason(reason string) bool {
	switch reason {
	case "stall_restart_start_stall", "stall_restart_running_stall", "instant_stall", "fail_restart", "timeout_restart":
		return true
	}
	return false
}
