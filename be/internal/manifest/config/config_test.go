package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "tool_manifest.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

const validManifestYAML = `
tools:
  - name: lookup_sku
    type: python_script
    description: Look up product details by SKU
    script: tools/lookup_sku.py
    input_schema:
      type: object
      properties:
        sku:
          type: string
      required:
        - sku
      additionalProperties: false
    config_files:
      - path: catalog.yaml
`

func TestLoad_ValidManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, validManifestYAML)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Tools) != 1 {
		t.Errorf("Tools len = %d, want 1", len(m.Tools))
	}
	if m.Dir != dir {
		t.Errorf("Dir = %q, want %q", m.Dir, dir)
	}
	tool, ok := m.Tool("lookup_sku")
	if !ok {
		t.Fatal("Tool('lookup_sku') not found")
	}
	if tool.Type != TypePythonScript {
		t.Errorf("Type = %v, want TypePythonScript", tool.Type)
	}
	if tool.Script != "tools/lookup_sku.py" {
		t.Errorf("Script = %q, want 'tools/lookup_sku.py'", tool.Script)
	}
	if len(tool.InputSchema) == 0 {
		t.Error("InputSchema is empty, want JSON bytes")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load missing file: expected error, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "{ invalid yaml ::::")
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load invalid YAML: expected error, got nil")
	}
}

func TestLoad_ToolTypeRejections(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		wantErr string
	}{
		{"builtin", "builtin", "no longer supported"},
		{"config_template", "config_template", "no longer supported"},
		{"unknown", "unknown_type", "unknown type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, `
tools:
  - name: tool1
    type: `+tc.typ+`
    description: test tool
    script: script.py
    input_schema:
      type: object
`)
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load with type=%q: expected error, got nil", tc.typ)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoad_EmptyType(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
tools:
  - name: tool1
    description: test
    script: s.py
    input_schema:
      type: object
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load empty type: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Errorf("error = %q, want 'type is required'", err.Error())
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{
			"missing_name",
			`
tools:
  - type: python_script
    description: test
    script: s.py
    input_schema:
      type: object
`, "name is required",
		},
		{
			"missing_description",
			`
tools:
  - name: tool1
    type: python_script
    script: s.py
    input_schema:
      type: object
`, "description is required",
		},
		{
			"missing_script",
			`
tools:
  - name: tool1
    type: python_script
    description: test
    input_schema:
      type: object
`, "script is required",
		},
		{
			"missing_input_schema",
			`
tools:
  - name: tool1
    type: python_script
    description: test
    script: s.py
`, "input_schema is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, tc.manifest)
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load %s: expected error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoad_DuplicateToolName(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
tools:
  - name: same
    type: python_script
    description: first
    script: s.py
    input_schema:
      type: object
  - name: same
    type: python_script
    description: second
    script: s2.py
    input_schema:
      type: object
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load duplicate tool: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate tool name") {
		t.Errorf("error = %q, want 'duplicate tool name'", err.Error())
	}
}

func TestLoad_ScriptPathValidation(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		wantErr string
	}{
		{"abs_path", "/absolute/path.py", "relative"},
		{"parent_traversal", "../secret.py", "parent directories"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, `
tools:
  - name: tool1
    type: python_script
    description: test
    script: `+tc.script+`
    input_schema:
      type: object
`)
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load script=%q: expected error, got nil", tc.script)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	cases := []struct {
		path    string
		wantErr bool
	}{
		{"", true},
		{"/absolute", true},
		{"../secret", true},
		{"../../etc/passwd", true},
		{"tools/script.py", false},
		{"catalog.yaml", false},
		{"subdir/file.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.path+"_"+strings.ReplaceAll(tc.path, "/", "_"), func(t *testing.T) {
			err := safePath(tc.path)
			if (err != nil) != tc.wantErr {
				t.Errorf("safePath(%q) error = %v, wantErr = %v", tc.path, err, tc.wantErr)
			}
		})
	}
}

func TestManifest_Tool_NotFound(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, validManifestYAML)
	m, _ := Load(dir)

	_, ok := m.Tool("nonexistent")
	if ok {
		t.Error("Tool('nonexistent') found, want not found")
	}
}

func TestManifest_ConfigPath(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, validManifestYAML)
	m, _ := Load(dir)

	tool, _ := m.Tool("lookup_sku")
	if tool.ConfigPath() != "catalog.yaml" {
		t.Errorf("ConfigPath = %q, want 'catalog.yaml'", tool.ConfigPath())
	}
}

func TestManifest_ConfigPath_Empty(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
tools:
  - name: no_config
    type: python_script
    description: no config files
    script: script.py
    input_schema:
      type: object
`)
	m, _ := Load(dir)
	tool, _ := m.Tool("no_config")
	if tool.ConfigPath() != "" {
		t.Errorf("ConfigPath empty tool = %q, want ''", tool.ConfigPath())
	}
}

func TestManifest_ValidateInput(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, validManifestYAML)
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := m.ValidateInput("lookup_sku", map[string]interface{}{"sku": "ABC-123"}); err != nil {
		t.Errorf("ValidateInput valid: %v", err)
	}

	if err := m.ValidateInput("lookup_sku", map[string]interface{}{}); err == nil {
		t.Error("ValidateInput missing 'sku': expected error, got nil")
	}

	if err := m.ValidateInput("lookup_sku", map[string]interface{}{"sku": "X", "extra": "Y"}); err == nil {
		t.Error("ValidateInput extra property: expected error, got nil")
	}
}

func TestManifest_ValidateInput_ToolNotFound(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, validManifestYAML)
	m, _ := Load(dir)

	err := m.ValidateInput("nonexistent", map[string]interface{}{})
	if err == nil {
		t.Fatal("ValidateInput unknown tool: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tool not found") {
		t.Errorf("error = %q, want 'tool not found'", err.Error())
	}
}

func TestManifest_ValidateInput_OptionalFields(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
tools:
  - name: optional
    type: python_script
    description: optional input
    script: script.py
    input_schema:
      type: object
      properties:
        x:
          type: string
`)
	m, _ := Load(dir)
	if err := m.ValidateInput("optional", map[string]interface{}{}); err != nil {
		t.Errorf("ValidateInput empty valid: %v", err)
	}
}
