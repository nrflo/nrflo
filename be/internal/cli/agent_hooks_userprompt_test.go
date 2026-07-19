package cli

import (
	"strings"
	"testing"
)

// TestRenderHookDecision_AdditionalContext is the golden test for a console
// UserPromptSubmit response: renderHookDecision must emit the
// hookSpecificOutput envelope with hookEventName="UserPromptSubmit" and
// additionalContext set to the injected digest.
func TestRenderHookDecision_AdditionalContext(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"additional_context":"digest"}`)
	got := renderHookDecision(resp)
	// json.Marshal sorts map[string]interface{} keys alphabetically:
	// additionalContext < hookEventName.
	want := `{"hookSpecificOutput":{"additionalContext":"digest","hookEventName":"UserPromptSubmit"}}`
	if got != want {
		t.Errorf("renderHookDecision =\n  %s\nwant:\n  %s", got, want)
	}
}

func TestRenderHookDecision_AdditionalContextAbsent_ReturnsEmpty(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"status":"recorded"}`)
	if got := renderHookDecision(resp); got != "" {
		t.Errorf("renderHookDecision = %q, want empty when additional_context is absent", got)
	}
}

func TestRenderHookDecision_AdditionalContextEmptyString_ReturnsEmpty(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"additional_context":""}`)
	if got := renderHookDecision(resp); got != "" {
		t.Errorf("renderHookDecision = %q, want empty when additional_context is the empty string", got)
	}
}

// TestRenderHookDecision_PermissionDecisionTakesPrecedenceOverAdditionalContext
// verifies a PreToolUse permission_decision response never leaks
// additionalContext even if both fields happen to be set — precedence stays
// PermissionDecision > StopDecision > AdditionalContext.
func TestRenderHookDecision_PermissionDecisionTakesPrecedenceOverAdditionalContext(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"permission_decision":{"decision":"allow","reason":"ok"},"additional_context":"should not appear"}`)
	got := renderHookDecision(resp)
	if !strings.Contains(got, "permissionDecision") || strings.Contains(got, "should not appear") {
		t.Errorf("permission_decision must take precedence over additional_context, got %s", got)
	}
}

// TestRenderHookDecision_StopDecisionTakesPrecedenceOverAdditionalContext
// mirrors the above for the Stop-hook block path.
func TestRenderHookDecision_StopDecisionTakesPrecedenceOverAdditionalContext(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"stop_decision":{"block":true,"reason":"keep going"},"additional_context":"should not appear"}`)
	got := renderHookDecision(resp)
	want := `{"decision":"block","reason":"keep going"}`
	if got != want {
		t.Errorf("renderHookDecision =\n  %s\nwant:\n  %s", got, want)
	}
}
