package usagelimits

import "time"

// UsageLimits holds usage data for all CLI tools.
type UsageLimits struct {
	Claude    ToolUsage `json:"claude"`
	Codex     ToolUsage `json:"codex"`
	FetchedAt time.Time `json:"fetched_at"`
}

// ToolUsage holds usage data for a single CLI tool.
type ToolUsage struct {
	Available bool         `json:"available"`
	Session   *UsageMetric `json:"session"`
	Weekly    *UsageMetric `json:"weekly"`
	Error     string       `json:"error,omitempty"`
}

// UsageMetric holds a single usage metric (session or weekly).
type UsageMetric struct {
	UsedPct  float64 `json:"used_pct"`
	ResetsAt string  `json:"resets_at"`
}
