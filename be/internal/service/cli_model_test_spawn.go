package service

// TestCLIModelResult holds the result of a CLI model health check
type TestCLIModelResult struct {
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}
