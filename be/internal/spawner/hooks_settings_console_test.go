package spawner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildConsoleSettingsJSON_PreToolUseTimeoutAndCommand(t *testing.T) {
	got := BuildConsoleSettingsJSON("/opt/nrflo_server")
	if got == "" {
		t.Fatal("expected non-empty JSON")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\ngot: %s", err, got)
	}
	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("missing top-level 'hooks' key")
	}
	pre, _ := hooks["PreToolUse"].([]interface{})
	if len(pre) != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1", len(pre))
	}
	entry, _ := pre[0].(map[string]interface{})
	innerHooks, _ := entry["hooks"].([]interface{})
	inner, _ := innerHooks[0].(map[string]interface{})
	cmd, _ := inner["command"].(string)
	if cmd != "/opt/nrflo_server agent record-event --console" {
		t.Errorf("PreToolUse command = %q, want %q", cmd, "/opt/nrflo_server agent record-event --console")
	}
	timeout, ok := inner["timeout"].(float64)
	if !ok || int(timeout) != consolePreToolUseTimeoutSec {
		t.Errorf("PreToolUse timeout = %v, want %d", inner["timeout"], consolePreToolUseTimeoutSec)
	}
}

func TestBuildConsoleSettingsJSON_OtherHooksHaveNoTimeoutAndCorrectCommand(t *testing.T) {
	got := BuildConsoleSettingsJSON("/opt/nrflo_server")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks, _ := parsed["hooks"].(map[string]interface{})

	for _, key := range []string{"PostToolUse", "UserPromptSubmit", "Notification", "SubagentStop", "PreCompact", "Stop", "SessionStart"} {
		entries, ok := hooks[key].([]interface{})
		if !ok || len(entries) == 0 {
			t.Fatalf("hooks.%s missing or empty (hooks: %v)", key, hooks)
		}
		entry, _ := entries[0].(map[string]interface{})
		innerHooks, _ := entry["hooks"].([]interface{})
		inner, _ := innerHooks[0].(map[string]interface{})
		if _, has := inner["timeout"]; has {
			t.Errorf("hooks.%s must not carry a timeout field, got %v", key, inner["timeout"])
		}
		cmd, _ := inner["command"].(string)
		if !strings.HasSuffix(cmd, "agent record-event --console") {
			t.Errorf("hooks.%s command = %q, want suffix %q", key, cmd, "agent record-event --console")
		}
	}
}

// TestBuildConsoleSettingsJSON_HookKeySetMatchesInteractive pins the acceptance
// requirement that the console hook set is the exact same event set as the
// autonomous BuildInteractiveSettingsJSON — only the command/timeout differ.
func TestBuildConsoleSettingsJSON_HookKeySetMatchesInteractive(t *testing.T) {
	consoleJSON := BuildConsoleSettingsJSON("/opt/nrflo_server")
	interactiveJSON := BuildInteractiveSettingsJSON(&processInfo{modelID: "claude:opus"})

	var consoleParsed, interactiveParsed map[string]interface{}
	if err := json.Unmarshal([]byte(consoleJSON), &consoleParsed); err != nil {
		t.Fatalf("invalid console JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(interactiveJSON), &interactiveParsed); err != nil {
		t.Fatalf("invalid interactive JSON: %v", err)
	}
	consoleHooks, _ := consoleParsed["hooks"].(map[string]interface{})
	interactiveHooks, _ := interactiveParsed["hooks"].(map[string]interface{})

	if len(consoleHooks) != len(interactiveHooks) {
		t.Fatalf("hook key count differs: console=%d interactive=%d", len(consoleHooks), len(interactiveHooks))
	}
	for key := range interactiveHooks {
		if _, ok := consoleHooks[key]; !ok {
			t.Errorf("console settings missing hook key %q present in interactive settings", key)
		}
	}
}

func TestBuildConsoleSettingsJSON_StatusLine(t *testing.T) {
	got := BuildConsoleSettingsJSON("/opt/nrflo_server")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	sl, ok := parsed["statusLine"].(map[string]interface{})
	if !ok {
		t.Fatal("missing statusLine")
	}
	if sl["type"] != "command" {
		t.Errorf("statusLine.type = %v, want command", sl["type"])
	}
	cmd, _ := sl["command"].(string)
	if !strings.HasSuffix(cmd, "agent statusline") {
		t.Errorf("statusLine.command = %q, want suffix 'agent statusline'", cmd)
	}
}

// TestBuildConsoleSettingsJSON_NoUnexpectedTopLevelKeys guards against ever
// merging a safety-hook (or any other) block into console settings.
func TestBuildConsoleSettingsJSON_NoUnexpectedTopLevelKeys(t *testing.T) {
	got := BuildConsoleSettingsJSON("/opt/nrflo_server")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for key := range parsed {
		if key != "hooks" && key != "statusLine" {
			t.Errorf("unexpected top-level key %q — console settings must never carry a safety-hook merge", key)
		}
	}
}

// TestBuildInteractiveSettingsJSON_PreToolUseHasNoTimeoutField pins the
// AUTONOMOUS UNCHANGED acceptance test: BuildInteractiveSettingsJSON must
// stay byte-shape-identical (no numeric timeout, no --console) now that
// BuildConsoleSettingsJSON exists.
func TestBuildInteractiveSettingsJSON_PreToolUseHasNoTimeoutField(t *testing.T) {
	proc := &processInfo{modelID: "claude:sonnet"}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(BuildInteractiveSettingsJSON(proc)), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks, _ := parsed["hooks"].(map[string]interface{})
	pre, _ := hooks["PreToolUse"].([]interface{})
	entry, _ := pre[0].(map[string]interface{})
	innerHooks, _ := entry["hooks"].([]interface{})
	inner, _ := innerHooks[0].(map[string]interface{})
	if _, has := inner["timeout"]; has {
		t.Errorf("BuildInteractiveSettingsJSON PreToolUse must not carry a numeric timeout, got %v", inner["timeout"])
	}
	cmd, _ := inner["command"].(string)
	if strings.Contains(cmd, "--console") {
		t.Errorf("BuildInteractiveSettingsJSON command must not carry --console, got %q", cmd)
	}
}
