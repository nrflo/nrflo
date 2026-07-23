package spawner

import "testing"

// TestAutoApproveWire pins autoApproveWire's method->wire mapping: only the
// two v2 approval-shaped methods resolve, and only to "accept" — never a
// legacy or unmapped string.
func TestAutoApproveWire(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		method   string
		wantWire string
		wantOK   bool
	}{
		{"v2_command_execution", "item/commandExecution/requestApproval", "accept", true},
		{"v2_file_change", "item/fileChange/requestApproval", "accept", true},
		{"legacy_exec_command_approval", "execCommandApproval", "", false},
		{"legacy_apply_patch_approval", "applyPatchApproval", "", false},
		{"permissions_not_decision_shaped", "item/permissions/requestApproval", "", false},
		{"unknown_method", "item/tool/requestUserInput", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, ok := autoApproveWire(tc.method)
			if ok != tc.wantOK {
				t.Fatalf("autoApproveWire(%q) ok = %v, want %v", tc.method, ok, tc.wantOK)
			}
			if ok && wire != tc.wantWire {
				t.Errorf("autoApproveWire(%q) wire = %q, want %q", tc.method, wire, tc.wantWire)
			}
		})
	}
}
