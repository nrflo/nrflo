package model

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// TestProjectMarshalJSON_PushAfterMergeAlwaysPresent verifies that push_after_merge
// is always serialized in the JSON output even when false (no omitempty).
func TestProjectMarshalJSON_PushAfterMergeAlwaysPresent(t *testing.T) {
	cases := []struct {
		name  string
		value bool
		want  bool
	}{
		{"false", false, false},
		{"true", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Project{
				ID:             "proj-1",
				Name:           "Test",
				PushAfterMerge: tc.value,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			data, err := json.Marshal(p)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			raw, ok := m["push_after_merge"]
			if !ok {
				t.Fatalf("push_after_merge absent from JSON output (value=%v)", tc.value)
			}
			got, ok := raw.(bool)
			if !ok {
				t.Fatalf("push_after_merge type = %T, want bool", raw)
			}
			if got != tc.want {
				t.Errorf("push_after_merge = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProjectMarshalJSON_PushAfterMergeIndependentOfSafetyHook verifies that
// PushAfterMerge serializes independently of ClaudeSafetyHook.
func TestProjectMarshalJSON_PushAfterMergeIndependentOfSafetyHook(t *testing.T) {
	p := Project{
		ID:               "proj-2",
		Name:             "Test",
		PushAfterMerge:   true,
		ClaudeSafetyHook: "",
		DefaultBranch:    sql.NullString{String: "main", Valid: true},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// push_after_merge must be true
	if val, ok := m["push_after_merge"].(bool); !ok || !val {
		t.Errorf("push_after_merge = %v, want true", m["push_after_merge"])
	}

	// claude_safety_hook must be null when empty
	if m["claude_safety_hook"] != nil {
		t.Errorf("claude_safety_hook = %v, want nil", m["claude_safety_hook"])
	}

	// default_branch must be present
	if m["default_branch"] != "main" {
		t.Errorf("default_branch = %v, want 'main'", m["default_branch"])
	}
}
