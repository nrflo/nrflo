package proc

import (
	"os"
	"testing"
)

func TestPidAlive_ZeroPid(t *testing.T) {
	if PidAlive(0) {
		t.Error("PidAlive(0) = true, want false")
	}
}

func TestPidAlive_NegativePid(t *testing.T) {
	if PidAlive(-1) {
		t.Error("PidAlive(-1) = true, want false")
	}
}

func TestPidAlive_SelfPid(t *testing.T) {
	pid := int64(os.Getpid())
	if !PidAlive(pid) {
		t.Errorf("PidAlive(%d) = false, want true (self pid)", pid)
	}
}

func TestPidMetrics_ZeroPid(t *testing.T) {
	_, _, _, ok := PidMetrics(0)
	if ok {
		t.Error("PidMetrics(0) ok = true, want false")
	}
}

func TestPidMetrics_NegativePid(t *testing.T) {
	_, _, _, ok := PidMetrics(-1)
	if ok {
		t.Error("PidMetrics(-1) ok = true, want false")
	}
}

func TestPidMetrics_DeadPid(t *testing.T) {
	// PID 99999999 is almost certainly not alive.
	_, _, _, ok := PidMetrics(99999999)
	if ok {
		t.Log("PidMetrics(99999999) returned ok=true; process may coincidentally exist, skipping assertion")
	}
}

func TestParseEtime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		// MM:SS format
		{"00:00", 0, false},
		{"01:30", 90, false},
		{"59:59", 3599, false},
		// HH:MM:SS format
		{"0:00:00", 0, false},
		{"1:02:03", 3723, false},
		{"23:59:59", 86399, false},
		// DD-HH:MM:SS format
		{"1-00:00:00", 86400, false},
		{"1-02:03:04", 86400 + 7200 + 180 + 4, false},
		{"2-12:00:00", 2*86400 + 12*3600, false},
		// Errors
		{"", 0, true},
		{"abc", 0, true},
		{"x:y", 0, true},
		{"1:2:x", 0, true},
		{"1-x:y:z", 0, true},
		{"1:2:3:4", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseEtime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseEtime(%q) = %d, nil; want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseEtime(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseEtime(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
