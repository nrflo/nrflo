package socket

import "testing"

func TestDenyGlobalWorkflowMutation(t *testing.T) {
	// Non-global project: not denied.
	if _, denied := denyGlobalWorkflowMutation(Request{ID: "1"}, "proj1"); denied {
		t.Errorf("proj1: denied=true, want false")
	}
	// Empty project: not denied (resolution happens upstream).
	if _, denied := denyGlobalWorkflowMutation(Request{ID: "1"}, ""); denied {
		t.Errorf("empty project: denied=true, want false")
	}
	// Global project (case-insensitive): denied with an error response.
	for _, pid := range []string{"__global__", "__GLOBAL__"} {
		resp, denied := denyGlobalWorkflowMutation(Request{ID: "1"}, pid)
		if !denied {
			t.Errorf("%s: denied=false, want true", pid)
		}
		if resp.Error == nil {
			t.Errorf("%s: expected an error response, got %+v", pid, resp)
		}
	}
}
