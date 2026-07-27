package spawner

import (
	"testing"
	"time"
)

// TestSpawnDeadlineIsMinutes pins the unit of the agent definition `timeout`
// column against the two spawn wrappers that used to open-code it in seconds.
// The regression it guards: dynamic-planner is seeded timeout=30, meaning 30
// minutes, and a seconds reading killed every planner spawn at T+30s.
func TestSpawnDeadlineIsMinutes(t *testing.T) {
	fallback := 10 * time.Minute
	tests := []struct {
		name    string
		timeout int
		want    time.Duration
	}{
		{"dynamic-planner seed", 30, 30 * time.Minute},
		{"planner-system seed", 10, 10 * time.Minute},
		{"unset falls back", 0, fallback},
		{"negative falls back", -1, fallback},
	}
	for _, tt := range tests {
		if got := SpawnDeadline(tt.timeout, fallback); got != tt.want {
			t.Errorf("%s: SpawnDeadline(%d) = %v, want %v", tt.name, tt.timeout, got, tt.want)
		}
	}
}

// TestSpawnDeadlineMatchesProcessBudget asserts the property that was actually
// violated: the context deadline a wrapper derives from a definition's timeout
// must not be shorter than the child process budget spawnerPrepare derives
// from the same value, or the outer deadline kills a child that believes it
// has far longer to run.
func TestSpawnDeadlineMatchesProcessBudget(t *testing.T) {
	for _, timeout := range []int{3, 5, 10, 30, 40} {
		processBudget := time.Duration(timeout) * time.Minute // spawnerPrepare's conversion
		if got := SpawnDeadline(timeout, 0); got < processBudget {
			t.Errorf("timeout=%d: deadline %v shorter than process budget %v", timeout, got, processBudget)
		}
	}
}
