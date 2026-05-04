package api

import (
	"testing"
)

func TestDiffJSON_AddedRemovedChanged(t *testing.T) {
	input := `{"a":1,"b":2}`
	draft := `{"a":1,"b":3,"c":4}`
	result := diffJSON(input, draft)
	if result == nil {
		t.Fatal("diffJSON returned nil")
	}
	if len(result.Removed) != 0 {
		t.Errorf("Removed = %v, want empty", result.Removed)
	}
	if _, ok := result.Added["c"]; !ok {
		t.Errorf("Added missing key 'c'; got %v", result.Added)
	}
	if _, ok := result.Changed["b"]; !ok {
		t.Errorf("Changed missing key 'b'; got %v", result.Changed)
	}
}

func TestDiffJSON_RemovedKey(t *testing.T) {
	result := diffJSON(`{"a":1,"b":2}`, `{"a":1}`)
	if result == nil {
		t.Fatal("diffJSON returned nil")
	}
	if _, ok := result.Removed["b"]; !ok {
		t.Errorf("Removed missing key 'b'; got %v", result.Removed)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want empty", result.Added)
	}
}

func TestDiffJSON_NilOnInvalidInput(t *testing.T) {
	cases := []struct {
		input string
		draft string
	}{
		{"not-json", `{"a":1}`},
		{`{"a":1}`, "not-json"},
		{"", ""},
	}
	for _, c := range cases {
		result := diffJSON(c.input, c.draft)
		if result != nil {
			t.Errorf("diffJSON(%q, %q) = %v, want nil", c.input, c.draft, result)
		}
	}
}

func TestDiffJSON_NilWhenInputNotObject(t *testing.T) {
	// JSON arrays are not objects — diffJSON should return nil
	result := diffJSON(`[1,2,3]`, `[1,2,3]`)
	if result != nil {
		t.Errorf("diffJSON(array, array) = %v, want nil", result)
	}
}

func TestDiffJSON_IdenticalObjects(t *testing.T) {
	result := diffJSON(`{"a":1}`, `{"a":1}`)
	if result == nil {
		t.Fatal("diffJSON returned nil for identical objects")
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.Changed) != 0 {
		t.Errorf("Expected empty diff; got added=%v removed=%v changed=%v",
			result.Added, result.Removed, result.Changed)
	}
}

func TestDiffJSON_EmptyObjects(t *testing.T) {
	result := diffJSON(`{}`, `{}`)
	if result == nil {
		t.Fatal("diffJSON returned nil for empty objects")
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.Changed) != 0 {
		t.Errorf("Expected empty diff for empty objects; got %v", result)
	}
}
