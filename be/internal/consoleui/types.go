package consoleui

import "encoding/json"

type Config struct {
	BaseURL string
	Token   string
	Project string
	Session string
}

type ChatDetail struct {
	SessionID        string     `json:"session_id"`
	Engine           string     `json:"engine"`
	Model            string     `json:"model"`
	ProjectID        string     `json:"project_id"`
	Status           string     `json:"status"`
	Turn             string     `json:"turn"`
	WorkDir          string     `json:"work_dir"`
	ContextLeft      *int       `json:"context_left"`
	Live             bool       `json:"live"`
	PendingApprovals []Approval `json:"pending_approvals"`
}

type Approval struct {
	ID      string `json:"approval_id"`
	Kind    string `json:"kind"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Reason  string `json:"reason"`
}

type Message struct {
	Content   string `json:"content"`
	Category  string `json:"category"`
	Payload   string `json:"payload,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Event struct {
	Type      string                     `json:"type"`
	ProjectID string                     `json:"project_id"`
	SessionID string                     `json:"session_id,omitempty"`
	Data      map[string]json.RawMessage `json:"data,omitempty"`
}

type streamUpdate struct {
	Events    []Event
	Connected *bool
	Err       error
}

func eventString(ev Event, key string) string {
	var value string
	_ = json.Unmarshal(ev.Data[key], &value)
	return value
}

func eventInt(ev Event, key string) int {
	var value int
	_ = json.Unmarshal(ev.Data[key], &value)
	return value
}
