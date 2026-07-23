package spawner

import (
	"context"
	"regexp"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/spawner/apirun/provider"
)

var injectablePlaceholderRe = regexp.MustCompile(`\$\{[^}]+\}`)

// stdTemplateVars builds the standard ${VAR} map shared by the template body,
// the system-prompt-suffix injectable, and (for api-mode) the api-system-prompt
// injectable, so all three render from identical vars. nodeID falls back to
// agentType when unset (Preview, interactive L0 starts); modelID's model part
// falls back to "sonnet-5" when parseModelID yields none. extraVars is merged
// last (caller-injected vars win on key collision).
func stdTemplateVars(agentType, nodeID, ticketID, projectID, workflowName, parentSession, childSession, modelID string, extraVars map[string]string) map[string]string {
	_, model := parseModelID(modelID)
	if model == "" {
		model = "sonnet-5"
	}

	nodeVar := nodeID
	if nodeVar == "" {
		nodeVar = agentType
	}

	vars := map[string]string{
		"AGENT":          agentType,
		"NODE_ID":        nodeVar,
		"TICKET_ID":      ticketID,
		"PROJECT_ID":     projectID,
		"WORKFLOW":       workflowName,
		"PARENT_SESSION": parentSession,
		"CHILD_SESSION":  childSession,
		"MODEL_ID":       modelID,
		"MODEL":          model,
	}
	for k, v := range extraVars {
		vars[k] = v
	}
	return vars
}

// renderAPISystemPrompt renders the named api-mode system-prompt injectable,
// falling back to the caller-supplied constant when the row is missing or
// empty. Autonomous workers use "api-system-prompt" (seeded byte-identical to
// defaultAPISystemPrompt by migration 000177); the console uses its own
// "api-console-system-prompt" id, which is intentionally unseeded so a fresh DB
// falls back to the console-specific constants (consoleAPISystem/FSSystem).
func renderAPISystemPrompt(ctx context.Context, pool *db.Pool, id string, vars map[string]string, fallback string) string {
	body := renderInjectable(ctx, pool, id, vars)
	if strings.TrimSpace(body) == "" {
		return fallback
	}
	return body
}

// apiSystemPromptWithSuffix renders the api-system-prompt injectable (via
// renderAPISystemPrompt) and appends the already-rendered system-prompt-suffix
// when non-empty, matching CLI-mode's suffix behavior. overrideID, when
// non-empty, is the agent def's system_template_id: it is rendered and used in
// place of the api-system-prompt/fallback base when non-empty — the def/profile
// template wins over api mode's own default, same precedence as CLI mode.
func apiSystemPromptWithSuffix(ctx context.Context, pool *db.Pool, vars map[string]string, suffix, fallback, overrideID string, specs []provider.ToolSpec) string {
	sys := ""
	if overrideID != "" {
		sys = renderInjectable(ctx, pool, overrideID, vars)
	}
	if strings.TrimSpace(sys) == "" {
		sys = renderAPISystemPrompt(ctx, pool, "api-system-prompt", vars, fallback)
	}
	if strings.TrimSpace(suffix) != "" {
		sys = sys + "\n\n" + suffix
	}
	return appendDelegationGuidance(ctx, pool, sys, specs, vars)
}

// appendDelegationGuidance appends the readonly "delegation-guidance"
// injectable to sys when specs include the `delegate` tool, matching
// apiSystemPromptWithSuffix's "\n\n" join + TrimSpace-empty guards. Returns
// sys byte-identical when delegate is absent or the injectable renders empty,
// so defs without the tool see an unchanged prompt.
func appendDelegationGuidance(ctx context.Context, pool *db.Pool, sys string, specs []provider.ToolSpec, vars map[string]string) string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return AppendDelegationGuidanceForTools(ctx, pool, sys, names, vars)
}

// AppendDelegationGuidanceForTools is the exported form of
// appendDelegationGuidance for callers outside this package that hold a tool
// name list rather than []provider.ToolSpec (the console package, whose
// chat-spec seam gates on a console.Profile's Catalogue) — the single guard
// this delegate-membership + render + append logic lives in (project rule:
// no duplicated guard logic across seams).
func AppendDelegationGuidanceForTools(ctx context.Context, pool *db.Pool, sys string, toolNames []string, vars map[string]string) string {
	hasDelegate := false
	for _, name := range toolNames {
		if name == "delegate" {
			hasDelegate = true
			break
		}
	}
	if !hasDelegate {
		return sys
	}
	guidance := renderInjectable(ctx, pool, "delegation-guidance", vars)
	if strings.TrimSpace(guidance) == "" {
		return sys
	}
	return sys + "\n\n" + guidance
}

// resolveSystemPromptOverride prefers the agent def's own system_template_id
// (rendered as an injectable) over the global claude_system_prompt_override_enabled
// gate: a non-empty def/profile template wins outright; otherwise falls back
// to systemPromptOverrideFor's existing gate + mode default. def is the
// already-resolved agent definition (nil when not found) — callers resolve it
// once (loadTemplate) rather than each helper doing its own lookup.
func (s *Spawner) resolveSystemPromptOverride(def *model.AgentDefinition, model string, vars map[string]string) string {
	if def != nil && def.SystemTemplateID != "" {
		if rendered := s.expandInjectable(def.SystemTemplateID, vars); rendered != "" {
			return rendered
		}
	}
	return s.systemPromptOverrideFor(model, vars)
}

// RenderInjectable is the exported wrapper over renderInjectable for callers
// outside this package (the console package, which cannot import unexported
// spawner symbols) that already hold a *db.Pool.
func RenderInjectable(ctx context.Context, pool *db.Pool, id string, vars map[string]string) string {
	return renderInjectable(ctx, pool, id, vars)
}

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
	return renderInjectable(context.Background(), pool, id, vars)
}

// renderInjectable loads an injectable template from default_templates and
// expands vars, stripping any leftover ${...} placeholders that had no
// matching var. Returns "" with a warning log if the template is not found.
func renderInjectable(ctx context.Context, pool *db.Pool, id string, vars map[string]string) string {
	var body string
	err := pool.QueryRow(`SELECT template FROM default_templates WHERE id = ? AND type = 'injectable'`, id).Scan(&body)
	if err != nil {
		logger.Warn(ctx, "injectable template not found", "id", id, "error", err)
		return ""
	}

	for k, v := range vars {
		body = strings.ReplaceAll(body, "${"+k+"}", v)
	}

	body = injectablePlaceholderRe.ReplaceAllString(body, "")

	return body
}
