package spawner

import "be/internal/proc"

// PidAlive reports whether a process with the given pid is alive on this host.
// Returns false for pid <= 0.
func PidAlive(pid int64) bool { return proc.PidAlive(pid) }

// PidMetrics returns resource usage for the given pid.
// Returns zeros and ok=false on any failure.
func PidMetrics(pid int64) (rssKB int64, cpuPct float64, etimeSec int64, ok bool) {
	return proc.PidMetrics(pid)
}
