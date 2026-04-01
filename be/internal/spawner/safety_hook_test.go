package spawner

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildSafetySettingsJSON_EmptyInput returns "" for empty string
func TestBuildSafetySettingsJSON_EmptyInput(t *testing.T) {
	got := BuildSafetySettingsJSON("")
	if got != "" {
		t.Errorf("BuildSafetySettingsJSON(\"\") = %q, want \"\"", got)
	}
}

// TestBuildSafetySettingsJSON_InvalidJSON returns "" for unparseable input
func TestBuildSafetySettingsJSON_InvalidJSON(t *testing.T) {
	got := BuildSafetySettingsJSON("not valid json")
	if got != "" {
		t.Errorf("BuildSafetySettingsJSON(invalid) = %q, want \"\"", got)
	}
}

// TestBuildSafetySettingsJSON_Disabled returns "" when enabled=false
func TestBuildSafetySettingsJSON_Disabled(t *testing.T) {
	cfg := `{"enabled":false,"allow_git":true}`
	got := BuildSafetySettingsJSON(cfg)
	if got != "" {
		t.Errorf("BuildSafetySettingsJSON(disabled) = %q, want \"\"", got)
	}
}

// TestBuildSafetySettingsJSON_EnabledReturnsValidJSON validates the structure
func TestBuildSafetySettingsJSON_EnabledReturnsValidJSON(t *testing.T) {
	cfg := `{"enabled":true,"allow_git":false}`
	got := BuildSafetySettingsJSON(cfg)
	if got == "" {
		t.Fatal("BuildSafetySettingsJSON(enabled) returned empty string")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("BuildSafetySettingsJSON returned invalid JSON: %v\ngot: %s", err, got)
	}

	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing top-level 'hooks' key, got: %v", parsed)
	}
	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok || len(preToolUse) == 0 {
		t.Fatalf("missing or empty 'hooks.PreToolUse', got: %v", hooks)
	}
	entry, ok := preToolUse[0].(map[string]interface{})
	if !ok {
		t.Fatalf("PreToolUse[0] is not an object: %v", preToolUse[0])
	}
	if entry["matcher"] != "Bash" {
		t.Errorf("PreToolUse[0].matcher = %v, want 'Bash'", entry["matcher"])
	}
	hooksArr, ok := entry["hooks"].([]interface{})
	if !ok || len(hooksArr) == 0 {
		t.Fatalf("missing inner 'hooks' array in entry")
	}
	inner, ok := hooksArr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("inner hook is not an object: %v", hooksArr[0])
	}
	if inner["type"] != "command" {
		t.Errorf("inner hook type = %v, want 'command'", inner["type"])
	}
	cmd, ok := inner["command"].(string)
	if !ok || cmd == "" {
		t.Errorf("inner hook command is empty or missing")
	}
}

// TestBuildSafetySettingsJSON_ContainsDangerousPattern checks user patterns are in bash
func TestBuildSafetySettingsJSON_ContainsDangerousPattern(t *testing.T) {
	cfg := `{"enabled":true,"dangerous_patterns":["drop table","curl | bash"]}`
	got := BuildSafetySettingsJSON(cfg)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	for _, pat := range []string{"drop table", "curl | bash"} {
		if !strings.Contains(got, pat) {
			t.Errorf("output does not contain dangerous pattern %q:\n%s", pat, got)
		}
	}
}

// TestBuildSafetySettingsJSON_ContainsAllowedPaths checks allowed paths appear in bash
func TestBuildSafetySettingsJSON_ContainsAllowedPaths(t *testing.T) {
	cfg := `{"enabled":true,"rm_rf_allowed_paths":["/tmp","/var/cache","node_modules"]}`
	got := BuildSafetySettingsJSON(cfg)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	for _, p := range []string{"/tmp", "/var/cache", "node_modules"} {
		if !strings.Contains(got, p) {
			t.Errorf("output does not contain allowed path %q:\n%s", p, got)
		}
	}
}

// TestBuildSafetySettingsJSON_GitBlockWhenDisallowed checks git ops appear when allow_git=false
func TestBuildSafetySettingsJSON_GitBlockWhenDisallowed(t *testing.T) {
	cfg := `{"enabled":true,"allow_git":false}`
	got := BuildSafetySettingsJSON(cfg)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	for _, op := range []string{"git commit", "git push", "git merge", "git rebase"} {
		if !strings.Contains(got, op) {
			t.Errorf("output does not contain git block for %q:\n%s", op, got)
		}
	}
}

// TestBuildSafetySettingsJSON_GitAllowedWhenEnabled checks git ops absent when allow_git=true
func TestBuildSafetySettingsJSON_GitAllowedWhenEnabled(t *testing.T) {
	cfg := `{"enabled":true,"allow_git":true}`
	got := BuildSafetySettingsJSON(cfg)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	for _, op := range []string{"git commit", "git push"} {
		if strings.Contains(got, op) {
			t.Errorf("output should not contain git block %q when allow_git=true:\n%s", op, got)
		}
	}
}

// =============================================================================
// buildSafetyCommand tests
// =============================================================================

func TestBuildSafetyCommand_HardcodedDangerousRmPatterns(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true}
	cmd := buildSafetyCommand(cfg)

	for _, pattern := range []string{"rm -rf /", "rm -rf ~", "rm -rf .", "rm -rf .."} {
		if !strings.Contains(cmd, pattern) {
			t.Errorf("buildSafetyCommand missing hardcoded pattern %q:\n%s", pattern, cmd)
		}
	}
}

func TestBuildSafetyCommand_UserDangerousPatterns(t *testing.T) {
	cfg := SafetyHookConfig{
		Enabled:           true,
		DangerousPatterns: []string{"wget http", "nc -e"},
	}
	cmd := buildSafetyCommand(cfg)

	for _, pat := range cfg.DangerousPatterns {
		if !strings.Contains(cmd, pat) {
			t.Errorf("buildSafetyCommand missing user pattern %q:\n%s", pat, cmd)
		}
	}
}

func TestBuildSafetyCommand_EmptyDangerousPatterns(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true, DangerousPatterns: []string{}}
	cmd := buildSafetyCommand(cfg)
	// Should still contain hardcoded rm patterns and default exit 0
	if !strings.Contains(cmd, "rm -rf /") {
		t.Errorf("missing hardcoded rm -rf / pattern:\n%s", cmd)
	}
	if !strings.Contains(cmd, "exit 0") {
		t.Errorf("missing exit 0:\n%s", cmd)
	}
}

func TestBuildSafetyCommand_AbsoluteAllowedPath_PrefixMatch(t *testing.T) {
	cfg := SafetyHookConfig{
		Enabled:          true,
		RmRfAllowedPaths: []string{"/tmp/workspace"},
	}
	cmd := buildSafetyCommand(cfg)

	// Absolute path → prefix match: should appear with trailing '*'
	if !strings.Contains(cmd, "/tmp/workspace*") {
		t.Errorf("absolute allowed path /tmp/workspace should use prefix match (*/tmp/workspace*):\n%s", cmd)
	}
}

func TestBuildSafetyCommand_RelativeAllowedPath_BasenameMatch(t *testing.T) {
	cfg := SafetyHookConfig{
		Enabled:          true,
		RmRfAllowedPaths: []string{"node_modules"},
	}
	cmd := buildSafetyCommand(cfg)

	// Relative path → basename match via `basename "$TARGET"`
	if !strings.Contains(cmd, "basename") {
		t.Errorf("relative allowed path should use basename match:\n%s", cmd)
	}
	if !strings.Contains(cmd, "node_modules") {
		t.Errorf("allowed path node_modules missing:\n%s", cmd)
	}
}

func TestBuildSafetyCommand_NoAllowedPaths_BlocksAll(t *testing.T) {
	cfg := SafetyHookConfig{
		Enabled:          true,
		RmRfAllowedPaths: []string{},
	}
	cmd := buildSafetyCommand(cfg)

	// No allowed paths → block all rm -rf
	if !strings.Contains(cmd, "not in allowed paths") {
		t.Errorf("expected 'not in allowed paths' block when no allowed paths configured:\n%s", cmd)
	}
	// Should not contain ALLOWED=0 or ALLOWED=1 logic
	if strings.Contains(cmd, "ALLOWED=0") {
		t.Errorf("should not have ALLOWED logic when no allowed paths:\n%s", cmd)
	}
}

func TestBuildSafetyCommand_GitBlocked_WhenAllowGitFalse(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true, AllowGit: false}
	cmd := buildSafetyCommand(cfg)

	gitOps := []string{"git commit", "git push", "git merge", "git rebase", "git reset", "git clean"}
	for _, op := range gitOps {
		if !strings.Contains(cmd, op) {
			t.Errorf("expected git op %q to be blocked:\n%s", op, cmd)
		}
	}
}

func TestBuildSafetyCommand_GitAllowed_WhenAllowGitTrue(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true, AllowGit: true}
	cmd := buildSafetyCommand(cfg)

	// git write ops should not appear in the command
	for _, op := range []string{"git commit", "git push"} {
		if strings.Contains(cmd, op) {
			t.Errorf("git op %q should be absent when allow_git=true:\n%s", op, cmd)
		}
	}
}

func TestBuildSafetyCommand_StartsWithBashC(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true}
	cmd := buildSafetyCommand(cfg)

	if !strings.HasPrefix(cmd, "bash -c '") {
		t.Errorf("command should start with \"bash -c '\", got: %s", cmd[:min(40, len(cmd))])
	}
	if !strings.HasSuffix(cmd, "'") {
		t.Errorf("command should end with single quote, got suffix: %s", cmd[max(0, len(cmd)-20):])
	}
}

func TestBuildSafetyCommand_ReadsStdinViajq(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true}
	cmd := buildSafetyCommand(cfg)

	if !strings.Contains(cmd, "jq") {
		t.Errorf("command should use jq to parse stdin:\n%s", cmd)
	}
	if !strings.Contains(cmd, "tool_input.command") {
		t.Errorf("command should extract .tool_input.command:\n%s", cmd)
	}
}

func TestBuildSafetyCommand_DefaultExit0(t *testing.T) {
	cfg := SafetyHookConfig{Enabled: true, AllowGit: true}
	cmd := buildSafetyCommand(cfg)

	if !strings.Contains(cmd, "exit 0") {
		t.Errorf("command should have default exit 0:\n%s", cmd)
	}
}

func TestBuildSafetyCommand_MultipleAllowedPaths(t *testing.T) {
	cfg := SafetyHookConfig{
		Enabled:          true,
		RmRfAllowedPaths: []string{"/tmp", "/var/cache", "dist", "build"},
	}
	cmd := buildSafetyCommand(cfg)

	// Should have ALLOWED=0 logic since paths are provided
	if !strings.Contains(cmd, "ALLOWED=0") {
		t.Errorf("expected ALLOWED=0 init with multiple allowed paths:\n%s", cmd)
	}
	// All paths should appear
	for _, p := range cfg.RmRfAllowedPaths {
		if !strings.Contains(cmd, p) {
			t.Errorf("allowed path %q not found in command:\n%s", p, cmd)
		}
	}
}

