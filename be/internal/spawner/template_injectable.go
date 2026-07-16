package spawner

import (
	"context"
	"regexp"
	"strings"

	"be/internal/logger"
)

var injectablePlaceholderRe = regexp.MustCompile(`\$\{[^}]+\}`)

// systemPromptOverrideFor returns the expanded system-prompt injectable when the model
// belongs to Anthropic, supports CLI mode, and the global claude_system_prompt_override_enabled setting is
// on; returns "" otherwise. The setting is read freshly from the pool at spawn time.
func (s *Spawner) systemPromptOverrideFor(model string, vars map[string]string) string {
	cfg, ok := s.config.ModelConfigs[model]
	if !ok || cfg.Provider != "anthropic" || cfg.CLIModel == "" {
		return ""
	}
	pool := s.pool()
	if pool == nil {
		return ""
	}
	if val, _ := pool.GetConfig("claude_system_prompt_override_enabled"); val != "true" {
		return ""
	}
	return s.expandInjectable("system-prompt", vars)
}

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
