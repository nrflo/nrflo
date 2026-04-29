package spawner

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildInteractiveSettingsJSON_ClaudeReturnsJSON verifies a claude process yields valid JSON.
func TestBuildInteractiveSettingsJSON_ClaudeReturnsJSON(t *testing.T) {
	proc := &processInfo{modelID: "claude:sonnet"}
	got := BuildInteractiveSettingsJSON(proc)
	if got == "" {
		t.Fatal("expected non-empty JSON for claude process")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("BuildInteractiveSettingsJSON returned invalid JSON: %v\ngot: %s", err, got)
	}
}

// TestBuildInteractiveSettingsJSON_NonClaudeReturnsEmpty verifies non-claude processes yield "".
func TestBuildInteractiveSettingsJSON_NonClaudeReturnsEmpty(t *testing.T) {
	for _, modelID := range []string{
		"opencode:opencode_qwen36_plus_free",
		"codex:codex_gpt_high",
	} {
		proc := &processInfo{modelID: modelID}
		if got := BuildInteractiveSettingsJSON(proc); got != "" {
			t.Errorf("modelID=%q: expected empty string, got %q", modelID, got[:min(60, len(got))])
		}
	}
}

// TestBuildInteractiveSettingsJSON_HasPreAndPostToolUse verifies PreToolUse and PostToolUse are present.
func TestBuildInteractiveSettingsJSON_HasPreAndPostToolUse(t *testing.T) {
	proc := &processInfo{modelID: "claude:opus"}
	var parsed map[string]interface{}
	json.Unmarshal([]byte(BuildInteractiveSettingsJSON(proc)), &parsed)

	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("missing top-level 'hooks' key")
	}
	for _, key := range []string{"PreToolUse", "PostToolUse"} {
		arr, ok := hooks[key].([]interface{})
		if !ok || len(arr) == 0 {
			t.Errorf("hooks.%s missing or empty (hooks: %v)", key, hooks)
		}
	}
}

// TestBuildInteractiveSettingsJSON_NoUnwantedHookKeys verifies Stop/SessionEnd/UserPromptSubmit absent.
func TestBuildInteractiveSettingsJSON_NoUnwantedHookKeys(t *testing.T) {
	proc := &processInfo{modelID: "claude:opus"}
	var parsed map[string]interface{}
	json.Unmarshal([]byte(BuildInteractiveSettingsJSON(proc)), &parsed)
	hooks, _ := parsed["hooks"].(map[string]interface{})

	for _, banned := range []string{"Stop", "SessionEnd", "UserPromptSubmit", "SessionStart"} {
		if _, ok := hooks[banned]; ok {
			t.Errorf("hooks should not contain %q key (got: %v)", banned, hooks)
		}
	}
}

// TestBuildInteractiveSettingsJSON_CommandAndMatcherShape verifies each hook entry structure.
func TestBuildInteractiveSettingsJSON_CommandAndMatcherShape(t *testing.T) {
	proc := &processInfo{modelID: "claude:sonnet"}
	got := BuildInteractiveSettingsJSON(proc)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks, _ := parsed["hooks"].(map[string]interface{})

	for _, hookKey := range []string{"PreToolUse", "PostToolUse"} {
		entries, _ := hooks[hookKey].([]interface{})
		if len(entries) == 0 {
			t.Fatalf("%s: no entries", hookKey)
		}
		entry, _ := entries[0].(map[string]interface{})
		if entry["matcher"] != "*" {
			t.Errorf("%s[0].matcher = %v, want '*'", hookKey, entry["matcher"])
		}
		innerHooks, _ := entry["hooks"].([]interface{})
		if len(innerHooks) == 0 {
			t.Fatalf("%s[0].hooks is empty", hookKey)
		}
		inner, _ := innerHooks[0].(map[string]interface{})
		if inner["type"] != "command" {
			t.Errorf("%s[0].hooks[0].type = %v, want 'command'", hookKey, inner["type"])
		}
		cmd, _ := inner["command"].(string)
		if !strings.Contains(cmd, "agent record-event") {
			t.Errorf("%s command %q does not contain 'agent record-event'", hookKey, cmd)
		}
	}
}

// TestMergeInteractiveSettings_ConcatenatesPreToolUseArrays verifies arrays are concatenated when
// both sides define the same hook event key (e.g. PreToolUse).
func TestMergeInteractiveSettings_ConcatenatesPreToolUseArrays(t *testing.T) {
	safety := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo safety"}]}]}}`
	hooks := `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"nrflo agent record-event"}]}]}}`

	got := mergeInteractiveSettings(safety, hooks)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON after merge: %v\nmerged: %s", err, got)
	}
	mergedHooks, _ := parsed["hooks"].(map[string]interface{})
	pre, _ := mergedHooks["PreToolUse"].([]interface{})
	if len(pre) != 2 {
		t.Errorf("PreToolUse array length = %d, want 2 (safety + hooks concatenated)\nmerged: %s", len(pre), got)
	}
}

// TestBuildInteractiveSettingsJSON_HasStatusLine verifies that the top-level
// "statusLine" key is present for Claude agents with type=command and a command
// that ends with "agent statusline".
func TestBuildInteractiveSettingsJSON_HasStatusLine(t *testing.T) {
	proc := &processInfo{modelID: "claude:sonnet"}
	got := BuildInteractiveSettingsJSON(proc)
	if got == "" {
		t.Fatal("expected non-empty JSON for claude process")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\ngot: %s", err, got)
	}

	sl, ok := parsed["statusLine"].(map[string]interface{})
	if !ok {
		t.Fatalf("parsed[\"statusLine\"] missing or not a map; got: %v", parsed["statusLine"])
	}
	if sl["type"] != "command" {
		t.Errorf("statusLine.type = %v, want \"command\"", sl["type"])
	}
	cmd, _ := sl["command"].(string)
	if !strings.HasSuffix(cmd, "agent statusline") {
		t.Errorf("statusLine.command = %q, want suffix \"agent statusline\"", cmd)
	}
}

// TestBuildInteractiveSettingsJSON_NonClaudeNoStatusLine verifies that non-Claude
// processes do not return a statusLine key (they return empty string).
func TestBuildInteractiveSettingsJSON_NonClaudeNoStatusLine(t *testing.T) {
	for _, modelID := range []string{
		"opencode:opencode_qwen36_plus_free",
		"codex:codex_gpt_high",
	} {
		proc := &processInfo{modelID: modelID}
		if got := BuildInteractiveSettingsJSON(proc); got != "" {
			t.Errorf("modelID=%q: expected empty string (no statusLine for non-Claude), got %q", modelID, got[:min(60, len(got))])
		}
	}
}

// TestComputeContextLeftPct verifies the formula 100 - (totalUsed*100/maxCtx) with edge cases.
func TestComputeContextLeftPct(t *testing.T) {
	cases := []struct {
		totalUsed int
		maxCtx    int
		want      int
		name      string
	}{
		{0, 200000, 100, "no usage"},
		{100000, 200000, 50, "half used"},
		// 66000*100/200000 = 33 (integer division), so 100-33 = 67
		{66000, 200000, 67, "planner example"},
		{200000, 200000, 0, "fully used"},
		{250000, 200000, 0, "over-used clamped to 0"},
		// zero/negative maxCtx defaults to 200000
		{0, 0, 100, "zero maxCtx uses 200000"},
		{0, -1, 100, "negative maxCtx uses 200000"},
		{200000, 1000000, 80, "1M context window"},
	}
	for _, tc := range cases {
		got := ComputeContextLeftPct(tc.totalUsed, tc.maxCtx)
		if got != tc.want {
			t.Errorf("[%s] ComputeContextLeftPct(%d, %d) = %d, want %d",
				tc.name, tc.totalUsed, tc.maxCtx, got, tc.want)
		}
	}
}
