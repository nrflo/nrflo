package apirun

import (
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/spawner/apirun/provider"
)

// HTTPHandlerFactory builds an HTTP-backed ToolHandler from a tool definition.
// Defined as a function value so the registry can stay free of an import cycle
// with the tools_http subpackage.
type HTTPHandlerFactory func(def *model.ToolDefinition) ToolHandler

// ResolveRegistry returns the tool specs and handler map an api-mode agent
// should be spawned with. toolsCSV is the comma-separated patterns from the
// agent definition (`*`, `prefix.*`, or exact tool name). Empty CSV is a
// text-only agent — returns empty registry without error. Non-empty CSV that
// matches nothing is a config error.
//
// httpDefs is the set of in-scope HTTP tool definitions for the agent's
// project + workflow (the caller is expected to have filtered by scope before
// calling).
func ResolveRegistry(
	toolsCSV string,
	builtins map[string]ToolHandler,
	httpDefs []*model.ToolDefinition,
	httpFactory HTTPHandlerFactory,
) ([]provider.ToolSpec, Registry, error) {
	patterns := splitPatterns(toolsCSV)
	if len(patterns) == 0 {
		return nil, Registry{}, nil
	}

	// Build available pool: builtin names + HTTP def names. HTTP def names take
	// precedence is impossible — names are unique across the namespace and we
	// detect collisions explicitly.
	available := make(map[string]ToolHandler, len(builtins)+len(httpDefs))
	for name, h := range builtins {
		available[name] = h
	}
	for _, def := range httpDefs {
		if def == nil || def.Name == "" {
			continue
		}
		if _, exists := available[def.Name]; exists {
			return nil, nil, fmt.Errorf("tool name %q collides with builtin", def.Name)
		}
		available[def.Name] = httpFactory(def)
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

// matchAvailable returns the subset of `pool` whose names match the pattern.
// Supported forms:
//   - "*"          -> all
//   - "prefix.*"   -> names starting with "prefix."
//   - "prefix_*"   -> names starting with "prefix_"
//   - exact        -> exact name
func matchAvailable(pattern string, pool map[string]ToolHandler) map[string]ToolHandler {
	matched := map[string]ToolHandler{}
	if pattern == "*" {
		for name, h := range pool {
			matched[name] = h
		}
		return matched
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		for name, h := range pool {
			if strings.HasPrefix(name, prefix) {
				matched[name] = h
			}
		}
		return matched
	}
	if h, ok := pool[pattern]; ok {
		matched[pattern] = h
	}
	return matched
}
