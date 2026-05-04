package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads dir/tool_manifest.yaml and returns a validated Manifest.
func Load(dir string) (*Manifest, error) {
	manifestPath := filepath.Join(dir, "tool_manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", manifestPath, err)
	}

	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	m := &Manifest{
		Dir:         dir,
		toolsByName: make(map[string]*Tool),
	}

	seen := make(map[string]struct{})
	for i, rt := range raw.Tools {
		if err := validateTool(&rt); err != nil {
			return nil, fmt.Errorf("tool[%d] %q: %w", i, rt.Name, err)
		}
		if _, dup := seen[rt.Name]; dup {
			return nil, fmt.Errorf("duplicate tool name %q", rt.Name)
		}
		seen[rt.Name] = struct{}{}

		schemaJSON, err := compileSchema(rt.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input_schema: %w", rt.Name, err)
		}

		if rt.Script != "" {
			if err := safePath(rt.Script); err != nil {
				return nil, fmt.Errorf("tool %q script path: %w", rt.Name, err)
			}
		}

		for _, cf := range rt.ConfigFiles {
			if err := safePath(cf.Path); err != nil {
				return nil, fmt.Errorf("tool %q config_file path: %w", rt.Name, err)
			}
		}

		t := Tool{
			Name:        rt.Name,
			Type:        ToolType(rt.Type),
			Description: rt.Description,
			Script:      rt.Script,
			ConfigFiles: rt.ConfigFiles,
			InputSchema: schemaJSON,
			Review:      rt.Review,
			EnvAllow:    rt.EnvAllow,
		}
		m.Tools = append(m.Tools, t)
		m.toolsByName[t.Name] = &m.Tools[len(m.Tools)-1]
	}

	return m, nil
}

// safePath rejects empty, absolute, or parent-traversal paths.
func safePath(p string) error {
	if p == "" {
		return fmt.Errorf("path must not be empty")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("path must be relative, got %q", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must not traverse parent directories: %q", p)
	}
	return nil
}
