package config

// ToolType identifies the execution backend for a tool.
type ToolType string

const (
	TypePythonScript ToolType = "python_script"
)

// ConfigFile describes a configuration file managed by this tool.
type ConfigFile struct {
	Path       string `yaml:"path"`        // relative to config dir
	SchemaPath string `yaml:"schema_path"` // optional sidecar schema (relative)
}

// Tool is a single validated entry from the manifest.
type Tool struct {
	Name        string       `yaml:"name"`
	Type        ToolType     `yaml:"type"`
	Description string       `yaml:"description"`
	Script      string       `yaml:"script"`       // path relative to manifest dir
	ConfigFiles []ConfigFile `yaml:"config_files"`
	InputSchema []byte       // compiled JSON bytes from input_schema field
	Review      bool         `yaml:"review"`     // when true, successful invocations create a review item
	EnvAllow    []string     `yaml:"env_allow"`  // glob patterns scoping env vars passed to the script
}

// ConfigPath returns the primary config file path for this tool, if set.
func (t *Tool) ConfigPath() string {
	if len(t.ConfigFiles) > 0 {
		return t.ConfigFiles[0].Path
	}
	return ""
}

// Manifest is the parsed and validated tool_manifest.yaml.
type Manifest struct {
	Dir         string           // absolute path to the manifest directory
	Tools       []Tool           // validated tools
	toolsByName map[string]*Tool // index for O(1) lookup
}

// Tool returns the named tool, or nil + false if not found.
func (m *Manifest) Tool(name string) (*Tool, bool) {
	t, ok := m.toolsByName[name]
	return t, ok
}

// rawTool is the YAML decode target (before validation).
type rawTool struct {
	Name        string                 `yaml:"name"`
	Type        string                 `yaml:"type"`
	Description string                 `yaml:"description"`
	Script      string                 `yaml:"script"`
	ConfigFiles []ConfigFile           `yaml:"config_files"`
	InputSchema map[string]interface{} `yaml:"input_schema"`
	Review      bool                   `yaml:"review"`
	EnvAllow    []string               `yaml:"env_allow"`
}

// rawManifest is the top-level YAML decode target.
type rawManifest struct {
	Tools []rawTool `yaml:"tools"`
}
