package scaffold

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRun_MaterializesTemplate(t *testing.T) {
	outDir := t.TempDir()
	if err := Run([]string{"--out=" + outDir, "--name=TestCo"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	expected := []string{
		"tool_manifest.yaml",
		filepath.Join("tools", "lookup_sku.py"),
	}
	for _, rel := range expected {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("missing file %q: %v", rel, err)
		}
	}
}

func TestRun_OnlyPythonScriptTypes(t *testing.T) {
	outDir := t.TempDir()
	if err := Run([]string{"--out=" + outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "tool_manifest.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	content := string(data)

	// Deprecated types must not appear as tool type values
	if strings.Contains(content, "type: builtin") {
		t.Error("tool_manifest.yaml contains 'type: builtin'")
	}
	if strings.Contains(content, "type: config_template") {
		t.Error("tool_manifest.yaml contains 'type: config_template'")
	}

	// Tool-level type lines use exactly 4-space indent ("    type: <value>")
	toolTypeRe := regexp.MustCompile(`(?m)^    type:\s*(\S+)`)
	matches := toolTypeRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Error("no tool type entries found in tool_manifest.yaml")
	}
	for _, m := range matches {
		if m[1] != "python_script" {
			t.Errorf("tool type = %q, want 'python_script'", m[1])
		}
	}
}

func TestRun_RefusesNonEmptyDir(t *testing.T) {
	outDir := t.TempDir()
	os.WriteFile(filepath.Join(outDir, "existing.txt"), []byte("content"), 0644) //nolint

	err := Run([]string{"--out=" + outDir})
	if err == nil {
		t.Fatal("Run non-empty dir without --force: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error = %q, want to contain 'not empty'", err.Error())
	}
}

func TestRun_Force_OverwritesNonEmptyDir(t *testing.T) {
	outDir := t.TempDir()
	os.WriteFile(filepath.Join(outDir, "existing.txt"), []byte("content"), 0644) //nolint

	if err := Run([]string{"--out=" + outDir, "--force"}); err != nil {
		t.Fatalf("Run --force: %v", err)
	}
}

func TestRun_RequiresOut(t *testing.T) {
	err := Run([]string{})
	if err == nil {
		t.Fatal("Run without --out: expected error, got nil")
	}
}

func TestRun_PyFileMode_Executable(t *testing.T) {
	outDir := t.TempDir()
	if err := Run([]string{"--out=" + outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	scriptPath := filepath.Join(outDir, "tools", "lookup_sku.py")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat .py: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf(".py file mode = %04o, want executable (0755)", info.Mode().Perm())
	}
}

func TestRun_YamlFileMode_NotExecutable(t *testing.T) {
	outDir := t.TempDir()
	if err := Run([]string{"--out=" + outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	manifestPath := filepath.Join(outDir, "tool_manifest.yaml")
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat .yaml: %v", err)
	}
	if info.Mode()&0111 != 0 {
		t.Errorf(".yaml file mode = %04o, want non-executable (0644)", info.Mode().Perm())
	}
}

func TestRun_SubstitutesName(t *testing.T) {
	outDir := t.TempDir()
	if err := Run([]string{"--out=" + outDir, "--name=AcmeCorp"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "AcmeCorp") {
		t.Errorf("README.md does not contain 'AcmeCorp'; content:\n%s", content)
	}
	if strings.Contains(content, "{{Name}}") {
		t.Error("README.md still contains unreplaced {{Name}} token")
	}
}

func TestRun_DefaultNameIsBasename(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "my-customer")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := Run([]string{"--out=" + outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	readmePath := filepath.Join(outDir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(data), "my-customer") {
		t.Errorf("README.md does not contain default name 'my-customer'")
	}
}

func TestSubstitute_SingleToken(t *testing.T) {
	result, err := Substitute([]byte("Hello {{Name}}!"), map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if string(result) != "Hello World!" {
		t.Errorf("Substitute = %q, want 'Hello World!'", string(result))
	}
}

func TestSubstitute_MultipleTokens(t *testing.T) {
	result, err := Substitute(
		[]byte("{{Name}} at {{Place}}"),
		map[string]string{"Name": "Alice", "Place": "Earth"},
	)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if string(result) != "Alice at Earth" {
		t.Errorf("Substitute = %q, want 'Alice at Earth'", string(result))
	}
}

func TestSubstitute_NoTokens(t *testing.T) {
	input := []byte("plain text")
	result, err := Substitute(input, map[string]string{})
	if err != nil {
		t.Fatalf("Substitute no tokens: %v", err)
	}
	if string(result) != "plain text" {
		t.Errorf("Substitute = %q, want unchanged 'plain text'", string(result))
	}
}

func TestSubstitute_UnknownToken(t *testing.T) {
	_, err := Substitute([]byte("Hello {{Unknown}}"), map[string]string{})
	if err == nil {
		t.Fatal("Substitute unknown token: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Unknown") {
		t.Errorf("error = %q, want to mention 'Unknown'", err.Error())
	}
}

func TestSubstitute_EmptyInput(t *testing.T) {
	result, err := Substitute([]byte{}, map[string]string{})
	if err != nil {
		t.Fatalf("Substitute empty: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Substitute empty = %q, want empty", result)
	}
}

func TestIsText(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"file.yaml", true},
		{"file.yml", true},
		{"file.json", true},
		{"file.md", true},
		{"file.txt", true},
		{"file.py", true},
		{"file.sh", true},
		{"file.toml", true},
		{"file.cfg", true},
		{"file.ini", true},
		{"file.YAML", true},
		{"file.PY", true},
		{"file.MD", true},
		{"file.bin", false},
		{"file.exe", false},
		{"file.so", false},
		{"file", false},
		{"file.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isText(tc.name)
			if got != tc.want {
				t.Errorf("isText(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
