package apirun

import (
	"fmt"
	"strings"

	"be/internal/spawner/apirun/provider"
)

// ResolveRegistry returns the tool specs and handler map an agent should be
// spawned with. toolsCSV is the comma-separated patterns from the agent
// definition (`*`, `prefix*`, or exact tool name). Empty CSV is a text-only
// agent — returns empty registry without error. Non-empty CSV that matches
// nothing is a config error.
//
// pythonHandlers is the set of python_scripts kind=tool handlers for the
// project, composed after builtins.
func ResolveRegistry(
	toolsCSV string,
	builtins map[string]ToolHandler,
	pythonHandlers []ToolHandler,
) ([]provider.ToolSpec, Registry, error) {
	patterns := splitPatterns(toolsCSV)
	if len(patterns) == 0 {
		return nil, Registry{}, nil
	}

	// Build available pool: builtins → python tools.
	// Collision detection ensures names are unique across both sources.
	available := make(map[string]ToolHandler, len(builtins)+len(pythonHandlers))
	for name, h := range builtins {
		available[name] = h
	}

	// Python tools: collide with builtins → error.
	for _, h := range pythonHandlers {
		spec := h.Spec()
		if spec.Name == "" {
			continue
		}
		if _, exists := available[spec.Name]; exists {
			return nil, nil, fmt.Errorf("tool name %q collides with builtin", spec.Name)
		}
		available[spec.Name] = h
	}

	out := Registry{}
	specs := []provider.ToolSpec{}
	for _, pat := range patterns {
		matched := matchAvailable(pat, available)
		if len(matched) == 0 {
			return nil, nil, fmt.Errorf("no tools matched pattern %q", pat)
		}
		for name, h := range matched {
			if _, dup := out[name]; dup {
				continue
			}
			out[name] = h
			specs = append(specs, h.Spec())
		}
	}
	return specs, out, nil
}

func splitPatterns(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// MatchName reports whether name matches pattern.
// Supported forms:
//   - "*"        -> true (matches all)
//   - "prefix*"  -> strings.HasPrefix(name, prefix)
//   - exact      -> pattern == name
func MatchName(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	return pattern == name
}

// matchAvailable returns the subset of `pool` whose names match the pattern.
func matchAvailable(pattern string, pool map[string]ToolHandler) map[string]ToolHandler {
	matched := map[string]ToolHandler{}
	for name, h := range pool {
		if MatchName(pattern, name) {
			matched[name] = h
		}
	}
	return matched
}

// MergeBaseline ensures every name in baseline is present in the registry,
// pulling missing handlers from builtins. Socket-completion spawns
// (cli_interactive/codex/api-via-cli) use it so a restrictive tools CSV can
// never strip the lifecycle tools an agent needs to signal completion.
// Idempotent: names already present (and names absent from builtins) are left
// untouched.
func MergeBaseline(specs []provider.ToolSpec, handlers Registry, builtins map[string]ToolHandler, baseline []string) ([]provider.ToolSpec, Registry) {
	if handlers == nil {
		handlers = Registry{}
	}
	for _, name := range baseline {
		if _, ok := handlers[name]; ok {
			continue
		}
		h, ok := builtins[name]
		if !ok {
			continue
		}
		handlers[name] = h
		specs = append(specs, h.Spec())
	}
	return specs, handlers
}
