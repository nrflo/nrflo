package socket

import "testing"

// TestNormalizeGeminiHookEvent_Rewrites verifies that Gemini hook names are
// rewritten to their Claude equivalents in place.
func TestNormalizeGeminiHookEvent_Rewrites(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"BeforeTool", "PreToolUse"},
		{"AfterTool", "PostToolUse"},
		{"AfterAgent", "Stop"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			event := map[string]interface{}{"hook_event_name": tc.in}
			normalizeGeminiHookEvent(event)
			got, _ := event["hook_event_name"].(string)
			if got != tc.want {
				t.Errorf("normalizeGeminiHookEvent(%q) hook_event_name = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeGeminiHookEvent_Idempotent verifies that calling the normalizer
// on an already-normalized name leaves it unchanged.
func TestNormalizeGeminiHookEvent_Idempotent(t *testing.T) {
	cases := []string{"PreToolUse", "PostToolUse", "Stop"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			event := map[string]interface{}{"hook_event_name": name}
			normalizeGeminiHookEvent(event)
			normalizeGeminiHookEvent(event)
			got, _ := event["hook_event_name"].(string)
			if got != name {
				t.Errorf("double normalizeGeminiHookEvent(%q) = %q, want %q", name, got, name)
			}
		})
	}
}

// TestNormalizeGeminiHookEvent_PassThrough verifies that already-canonical and
// unknown names are left untouched.
func TestNormalizeGeminiHookEvent_PassThrough(t *testing.T) {
	cases := []string{
		"PreToolUse", "PostToolUse", "Stop",
		"SessionStart", "SessionEnd", "Notification", "Foo",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			event := map[string]interface{}{"hook_event_name": name}
			normalizeGeminiHookEvent(event)
			got, _ := event["hook_event_name"].(string)
			if got != name {
				t.Errorf("normalizeGeminiHookEvent(%q) = %q, want %q (pass-through)", name, got, name)
			}
		})
	}
}

// TestNormalizeGeminiHookEvent_Safety verifies the function does not panic and
// leaves the map unchanged for missing key or non-string value.
func TestNormalizeGeminiHookEvent_Safety(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		event := map[string]interface{}{}
		normalizeGeminiHookEvent(event) // must not panic
		if _, ok := event["hook_event_name"]; ok {
			t.Errorf("missing-key case: hook_event_name should not be set after normalize")
		}
	})

	t.Run("non-string value", func(t *testing.T) {
		event := map[string]interface{}{"hook_event_name": 42}
		normalizeGeminiHookEvent(event) // must not panic
		if v, _ := event["hook_event_name"].(int); v != 42 {
			t.Errorf("non-string case: hook_event_name = %v, want 42 (unchanged)", event["hook_event_name"])
		}
	})
}
