package service

import "testing"

func TestStopHookAllows(t *testing.T) {
	cases := []struct {
		name   string
		status string
		result string
		want   bool
	}{
		{"running no result -> block", "running", "", false},
		{"running with pass result -> allow", "running", "pass", true},
		{"running with fail result -> allow", "running", "fail", true},
		{"user_interactive -> allow", "user_interactive", "", true},
		{"waiting -> allow", "waiting", "", true},
		{"completed -> allow", "completed", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stopHookAllows(tc.status, tc.result); got != tc.want {
				t.Errorf("stopHookAllows(%q,%q) = %v, want %v", tc.status, tc.result, got, tc.want)
			}
		})
	}
}

func TestStopHookBudget(t *testing.T) {
	for count := 1; count <= stopBlockCap; count++ {
		block, capExceeded := stopHookBudget(count)
		if !block || capExceeded {
			t.Errorf("count %d: block=%v capExceeded=%v, want block=true cap=false", count, block, capExceeded)
		}
	}
	if block, capExceeded := stopHookBudget(stopBlockCap + 1); block || !capExceeded {
		t.Errorf("count %d: block=%v capExceeded=%v, want block=false cap=true", stopBlockCap+1, block, capExceeded)
	}
}
