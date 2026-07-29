package types

// DependencyRequest is the request for adding/removing dependencies
type DependencyRequest struct {
	Child  string `json:"child"`
	Parent string `json:"parent"`
}

// StatusRequest is the request for ticket status summary
type StatusRequest struct {
	PendingLimit   int `json:"pending_limit,omitempty"`
	CompletedLimit int `json:"completed_limit,omitempty"`
}
