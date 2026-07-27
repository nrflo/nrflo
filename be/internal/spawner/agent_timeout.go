package spawner

import "time"

// SpawnDeadline converts an agent definition's `timeout` column into a spawn
// context deadline. The column is MINUTES — the unit spawnerPrepare and
// spawnerScript already apply when they turn the same field into the child
// process budget, and the unit the agent form labels it with. Every caller
// that wraps a spawn in its own context must go through here: a caller that
// open-codes the conversion in seconds gives the child a deadline 60x shorter
// than the process budget derived from the very same value, and the outer
// deadline wins.
func SpawnDeadline(timeoutMinutes int, fallback time.Duration) time.Duration {
	if timeoutMinutes > 0 {
		return time.Duration(timeoutMinutes) * time.Minute
	}
	return fallback
}
