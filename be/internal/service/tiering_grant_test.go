package service

import "testing"

// TestGrantDelegationTools covers the empty-safe, order-preserving,
// idempotent-append behavior grantDelegationTools must guarantee: apply's
// idempotency (unchanged vs applied) and the hand-patched-def no-op case
// both depend on this function returning changed=false whenever both
// delegation tools are already present, in any position.
func TestGrantDelegationTools(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		existing    string
		wantCSV     string
		wantChanged bool
	}{
		{
			name:        "empty",
			existing:    "",
			wantCSV:     "delegate,get_delegation",
			wantChanged: true,
		},
		{
			name:        "partial_delegate_only",
			existing:    "delegate",
			wantCSV:     "delegate,get_delegation",
			wantChanged: true,
		},
		{
			name:        "already_both_plus_unrelated",
			existing:    "foo,delegate,get_delegation",
			wantCSV:     "foo,delegate,get_delegation",
			wantChanged: false,
		},
		{
			name:        "already_both_reversed_order",
			existing:    "get_delegation,delegate",
			wantCSV:     "get_delegation,delegate",
			wantChanged: false,
		},
		{
			name:        "hand_patched_exact_csv",
			existing:    "delegate,get_delegation",
			wantCSV:     "delegate,get_delegation",
			wantChanged: false,
		},
		{
			name:        "partial_get_delegation_only_preserves_unrelated",
			existing:    "bar,get_delegation,baz",
			wantCSV:     "bar,get_delegation,baz,delegate",
			wantChanged: true,
		},
		{
			name:        "whitespace_and_empty_entries_trimmed",
			existing:    " delegate , , get_delegation ",
			wantCSV:     "delegate,get_delegation",
			wantChanged: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCSV, gotChanged := grantDelegationTools(c.existing)
			if gotCSV != c.wantCSV {
				t.Errorf("grantDelegationTools(%q) csv = %q, want %q", c.existing, gotCSV, c.wantCSV)
			}
			if gotChanged != c.wantChanged {
				t.Errorf("grantDelegationTools(%q) changed = %v, want %v", c.existing, gotChanged, c.wantChanged)
			}
		})
	}
}
