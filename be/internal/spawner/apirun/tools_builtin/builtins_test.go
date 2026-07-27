package tools_builtin

import (
	"encoding/json"
	"testing"
)

// TestBuiltins_InputSchemasAreValidJSONObjects mirrors console/registry_test.go's
// catalogue check for the session-bound side: every builtin's InputSchema is a
// hand-written JSON literal that only reaches a provider at spawn time, so a
// malformed one (an unescaped quote in a long argument description) would
// otherwise surface as a runtime tool-registration failure instead of a test.
func TestBuiltins_InputSchemasAreValidJSONObjects(t *testing.T) {
	for name, h := range Builtins() {
		spec := h.Spec()
		if len(spec.InputSchema) == 0 {
			t.Errorf("%s: empty InputSchema", name)
			continue
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Errorf("%s: InputSchema does not unmarshal: %v", name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: InputSchema type = %v, want \"object\"", name, schema["type"])
		}
	}
}
