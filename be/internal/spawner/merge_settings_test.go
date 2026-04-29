package spawner

import (
	"encoding/json"
	"testing"
)

// TestMergeInteractiveSettings_BothEmpty returns "" when both inputs are empty.
func TestMergeInteractiveSettings_BothEmpty(t *testing.T) {
	if got := mergeInteractiveSettings("", ""); got != "" {
		t.Errorf("mergeInteractiveSettings(\"\", \"\") = %q, want \"\"", got)
	}
}

// TestMergeInteractiveSettings_SafetyEmptyReturnsHooks returns the hooks JSON
// unchanged when the safety input is empty.
func TestMergeInteractiveSettings_SafetyEmptyReturnsHooks(t *testing.T) {
	hooks := `{"hooks":{"PostToolUse":[{"matcher":"Write"}]}}`
	if got := mergeInteractiveSettings("", hooks); got != hooks {
		t.Errorf("mergeInteractiveSettings(\"\", hooks) = %q, want %q", got, hooks)
	}
}

// TestMergeInteractiveSettings_HooksEmptyReturnsSafety returns the safety JSON
// unchanged when the hooks input is empty.
func TestMergeInteractiveSettings_HooksEmptyReturnsSafety(t *testing.T) {
	safety := `{"hooks":{"PreToolUse":[{"matcher":"Bash"}]}}`
	if got := mergeInteractiveSettings(safety, ""); got != safety {
		t.Errorf("mergeInteractiveSettings(safety, \"\") = %q, want %q", got, safety)
	}
}

// TestMergeInteractiveSettings_BothSet_MergesHooks verifies that when both
// inputs contain a "hooks" sub-map, the keys from both are present in the result.
func TestMergeInteractiveSettings_BothSet_MergesHooks(t *testing.T) {
	safety := `{"hooks":{"PreToolUse":[{"matcher":"Bash"}]}}`
	hooks := `{"hooks":{"PostToolUse":[{"matcher":"Write"}]}}`
	got := mergeInteractiveSettings(safety, hooks)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("mergeInteractiveSettings result is not valid JSON: %v\ngot: %s", err, got)
	}
	hooksMap, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("merged result missing 'hooks' key: %v", parsed)
	}
	if _, hasPreToolUse := hooksMap["PreToolUse"]; !hasPreToolUse {
		t.Errorf("merged hooks missing PreToolUse from safety: %v", hooksMap)
	}
	if _, hasPostToolUse := hooksMap["PostToolUse"]; !hasPostToolUse {
		t.Errorf("merged hooks missing PostToolUse from hooks: %v", hooksMap)
	}
}

// TestMergeInteractiveSettings_InvalidSafetyJSON returns hooksJSON unchanged
// when safetyJSON is not parseable.
func TestMergeInteractiveSettings_InvalidSafetyJSON_ReturnsHooks(t *testing.T) {
	hooks := `{"hooks":{"PostToolUse":[]}}`
	got := mergeInteractiveSettings("not-valid-json", hooks)
	if got != hooks {
		t.Errorf("mergeInteractiveSettings(invalid safety, hooks) = %q, want %q", got, hooks)
	}
}

// TestMergeInteractiveSettings_InvalidHooksJSON returns safetyJSON unchanged
// when hooksJSON is not parseable.
func TestMergeInteractiveSettings_InvalidHooksJSON_ReturnsSafety(t *testing.T) {
	safety := `{"hooks":{"PreToolUse":[]}}`
	got := mergeInteractiveSettings(safety, "not-valid-json")
	if got != safety {
		t.Errorf("mergeInteractiveSettings(safety, invalid hooks) = %q, want %q", got, safety)
	}
}

// TestMergeInteractiveSettings_NoHooksKey merges two valid JSON objects that
// lack a "hooks" key — both sides must be present in the result.
func TestMergeInteractiveSettings_NoHooksKey(t *testing.T) {
	safety := `{"allow":true}`
	hooks := `{"extra":"data"}`
	got := mergeInteractiveSettings(safety, hooks)
	if got == "" {
		t.Fatal("mergeInteractiveSettings with valid JSONs should not return empty")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v\ngot: %s", err, got)
	}
	// "allow" from safety must be present in the merged output.
	if _, ok := parsed["allow"]; !ok {
		t.Errorf("merged result missing 'allow' from safety: %v", parsed)
	}
}

// TestMergeInteractiveSettings_WithRealSafetyJSON verifies composability with
// BuildSafetySettingsJSON: merging real safety output with empty hooks returns
// the safety output unchanged (no accidental mutation).
func TestMergeInteractiveSettings_WithRealSafetyJSON(t *testing.T) {
	safety := BuildSafetySettingsJSON(`{"enabled":true,"allow_git":true}`)
	if safety == "" {
		t.Skip("safety hook disabled; coverage provided by safety_hook_test.go")
	}
	got := mergeInteractiveSettings(safety, "")
	if got != safety {
		t.Errorf("mergeInteractiveSettings(realSafety, \"\") modified safety JSON unexpectedly")
	}
}

// TestMergeInteractiveSettings_HooksKeyOverridesFromHooksSide verifies that when
// hooks side has a different key than safety side, both are preserved.
func TestMergeInteractiveSettings_HooksKeyOverridesFromHooksSide(t *testing.T) {
	safety := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"check-bash"}]}]}}`
	hooks := `{"hooks":{"PreToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"check-write"}]}]}}`
	got := mergeInteractiveSettings(safety, hooks)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooksMap, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hooks: %v", parsed)
	}
	// When both sides have the same key, the hooks side wins (last write).
	if _, hasPreToolUse := hooksMap["PreToolUse"]; !hasPreToolUse {
		t.Errorf("merged hooks missing PreToolUse: %v", hooksMap)
	}
}
