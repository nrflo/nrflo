package consoleui

import (
	"encoding/json"

	"be/internal/types"
)

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
	Profile          string     `json:"profile,omitempty"`
	ProjectID        string     `json:"project_id"`
	Status           string     `json:"status"`
	Turn             string     `json:"turn"`
	WorkDir          string     `json:"work_dir"`
	ContextLeft      *int       `json:"context_left"`
	CostEstimate     *float64   `json:"cost_estimate"`
	Live             bool       `json:"live"`
	PendingApprovals []Approval `json:"pending_approvals"`
	SessionApprovals []string   `json:"session_approvals"`
	LiveItems        []LiveItem `json:"live_items"`
	Thinking         *LiveItem  `json:"thinking,omitempty"`
	Yolo             bool       `json:"yolo"`
	QueuedPrompts    []string   `json:"queued_prompts,omitempty"`
}

type LiveItem struct {
	ID   string `json:"item_id"`
	Text string `json:"text"`
}

type Approval struct {
	ID      string `json:"approval_id"`
	Kind    string `json:"kind"`
	Tool    string `json:"tool,omitempty"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Reason  string `json:"reason"`
	// Input is the verbatim tool-input JSON; for Tool=AskUserQuestion it
	// carries the questions array the interactive card renders (question.go).
	Input string `json:"input,omitempty"`
}

type Message struct {
	Content   string `json:"content"`
	Category  string `json:"category"`
	Payload   string `json:"payload,omitempty"`
	CreatedAt string `json:"created_at"`
}

type MessagePage struct {
	Messages []Message `json:"messages"`
	Total    int       `json:"total"`
}

type Selection struct {
	ResumeID string
	Engine   string
	Model    string
	// Effort is a create-time reasoning-effort override; "" inherits the
	// model row's configured effort.
	Effort string
}

type Catalog = types.ConsoleCatalog

// ConsoleTool mirrors the GET .../tools payload entry (consoleToolSummary in
// be/internal/api/handlers_console_tools.go).
type ConsoleTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// InvokeResult mirrors the POST .../invoke response body.
type InvokeResult struct {
	OK         bool   `json:"ok"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Informed   bool   `json:"informed"`
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

func eventFloat(ev Event, key string) float64 {
	var value float64
	_ = json.Unmarshal(ev.Data[key], &value)
	return value
}

func eventBool(ev Event, key string) bool {
	var value bool
	_ = json.Unmarshal(ev.Data[key], &value)
	return value
}

func eventStrings(ev Event, key string) []string {
	var value []string
	_ = json.Unmarshal(ev.Data[key], &value)
	return value
}
