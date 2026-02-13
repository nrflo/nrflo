package model

type DailyStats struct {
	ID             int64   `json:"id"`
	ProjectID      string  `json:"project_id"`
	Date           string  `json:"date"`
	TicketsCreated int     `json:"tickets_created"`
	TicketsClosed  int     `json:"tickets_closed"`
	TokensSpent    int64   `json:"tokens_spent"`
	AgentTimeSec   float64 `json:"agent_time_sec"`
	UpdatedAt      string  `json:"updated_at"`
}
