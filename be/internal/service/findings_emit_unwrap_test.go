package service

import (
	"encoding/json"
	"testing"
)

// TestUnwrapDoubleEncodedJSON covers the tolerance for CLI MCP clients that
// stringify object/array tool arguments, while leaving genuine values intact.
func TestUnwrapDoubleEncodedJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"double-encoded object", `"{\"question\":\"q\",\"angles\":[]}"`, `{"question":"q","angles":[]}`},
		{"double-encoded array", `"[{\"claimRef\":\"0\"}]"`, `[{"claimRef":"0"}]`},
		{"plain object untouched", `{"question":"q"}`, `{"question":"q"}`},
		{"plain array untouched", `[1,2,3]`, `[1,2,3]`},
		{"scalar string untouched", `"Paris"`, `"Paris"`},
		{"scalar number untouched", `42`, `42`},
		{"string that only looks like json untouched", `"{not really json}"`, `"{not really json}"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(unwrapDoubleEncodedJSON(json.RawMessage(tc.in)))
			if got != tc.want {
				t.Errorf("unwrap(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
