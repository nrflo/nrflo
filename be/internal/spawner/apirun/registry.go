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

// ManifestProvider is the contract for manifest-backed tools. The interface
// keeps registry.go free of a tools_manifest import (same pattern as HTTPHandlerFactory).
type ManifestProvider interface {
	Specs() []provider.ToolSpec
	Handler(name string) (ToolHandler, bool)
}

// ResolveRegistry returns the tool specs and handler map an api-mode agent
// should be spawned with. toolsCSV is the comma-separated patterns from the
// agent definition (`*`, `prefix.*`, or exact tool name). Empty CSV is a
// text-only agent — returns empty registry without error. Non-empty CSV that
// matches nothing is a config error.
//
// httpDefs is the set of in-scope HTTP tool definitions for the agent's
// project + workflow (the caller is expected to have filtered by scope before
// calling).
//
// manifestProvider is optional (nil = no manifest tools). When non-nil, its
// tools are composed as a third source after builtins and HTTP defs.
func ResolveRegistry(
	toolsCSV string,
	builtins map[string]ToolHandler,
	httpDefs []*model.ToolDefinition,
	httpFactory HTTPHandlerFactory,
	manifestProvider ManifestProvider,
) ([]provider.ToolSpec, Registry, error) {
	patterns := splitPatterns(toolsCSV)
	if len(patterns) == 0 {
		return nil, Registry{}, nil
	}

	// Build available pool: builtins → manifest tools → HTTP defs.
	// Collision detection ensures names are unique across all three sources.
	available := make(map[string]ToolHandler, len(builtins)+len(httpDefs))
	for name, h := range builtins {
		available[name] = h
	}

	// Manifest tools: collide with builtins → error.
	if manifestProvider != nil {
		for _, spec := range manifestProvider.Specs() {
			if spec.Name == "" {
				continue
			}
			if _, exists := available[spec.Name]; exists {
				return nil, nil, fmt.Errorf("tool name %q collides with builtin", spec.Name)
			}
			h, ok := manifestProvider.Handler(spec.Name)
			if !ok {
				continue
			}
			available[spec.Name] = h
		}
	}

	// HTTP tool defs: collide with builtins or manifest tools → error.
	for _, def := range httpDefs {
		if def == nil || def.Name == "" {
			continue
		}
		if _, exists := available[def.Name]; exists {
			// Could be a builtin or a manifest tool — report generically as collision.
			if _, isBuiltin := builtins[def.Name]; isBuiltin {
				return nil, nil, fmt.Errorf("tool name %q collides with builtin", def.Name)
			}
			return nil, nil, fmt.Errorf("tool name %q collides with manifest tool", def.Name)
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
