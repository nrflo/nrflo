package socket

import (
	"fmt"
	"strings"
	"testing"

	"be/internal/clock"
)

// fullContentCase describes a hook event and the expected stored content.
type fullContentCase struct {
	name    string
	event   map[string]interface{}
	want    string // expected content in agent_messages
	wantCat string
}

// buildFullContentCases constructs the five truncation-removed test cases.
// Each raw payload is intentionally longer than the old byte cap so a
// surviving cap would produce a wrong content (shorter or with "…").
func buildFullContentCases() []fullContentCase {
	prompt600 := strings.Repeat("p", 600) // old cap was 500 B
	prompt300 := strings.Repeat("q", 300) // old cap was 200 B
	err400 := strings.Repeat("e", 400)    // old cap was 300 B
	errBash400 := strings.Repeat("f", 400)
	body400 := strings.Repeat("g", 400)

	return []fullContentCase{
		{
			name:    "UserPromptSubmit",
			event:   map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": prompt600},
			want:    prompt600,
			wantCat: "user_input",
		},
		{
			name: "SubagentStart",
			event: map[string]interface{}{
				"hook_event_name": "SubagentStart",
				"agent_type":      "claude",
				"prompt":          prompt300,
			},
			want:    fmt.Sprintf("[claude] subagent started: %s", prompt300),
			wantCat: "subagent",
		},
		{
			name:    "StopFailure",
			event:   map[string]interface{}{"hook_event_name": "StopFailure", "error": err400},
			want:    "turn failed: " + err400,
			wantCat: "text",
		},
		{
			name: "PostToolUseFailure_error",
			event: map[string]interface{}{
				"hook_event_name": "PostToolUseFailure",
				"tool_name":       "Bash",
				"error":           errBash400,
			},
			want:    "[Bash failed] " + errBash400,
			wantCat: "tool",
		},
		{
			name: "PostToolUseFailure_body_fallback",
			event: map[string]interface{}{
				"hook_event_name": "PostToolUseFailure",
				"tool_name":       "Bash",
				// No "error" field — handler falls back to tool_response body.
				"tool_response": body400,
			},
			want:    "[Bash failed] " + body400,
			wantCat: "tool",
		},
	}
}

// TestRecordEvent_FullContent_NoTruncation verifies that content stored in
// agent_messages is never byte-capped. Payloads are intentionally larger than
// the removed caps so a surviving cap would produce a wrong (shorter) result.
func TestRecordEvent_FullContent_NoTruncation(t *testing.T) {
	for _, c := range buildFullContentCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			env := newHandlerTestEnv(t)
			ticketID := "RE-FULL-" + c.name
			env.createTicketAndWorkflow(t, ticketID)
			wfiID := queryWFIID(t, env, ticketID)
			sessionID := "sess-full-" + c.name
			insertAgentSession(t, env, ticketID, sessionID, wfiID)

			h := NewHandler(env.pool, env.hub, clock.Real(), nil)
			req := buildRecordEventReq(t, "req-full-"+c.name, sessionID, c.event)

			resp := h.Handle(req)
			if resp.Error != nil {
				t.Fatalf("%s: expected no error, got: %v", c.name, resp.Error)
			}

			if n := countAgentMessages(t, env, sessionID); n != 1 {
				t.Fatalf("%s: agent_messages count = %d, want 1", c.name, n)
			}

			content, category := lastAgentMessage(t, env, sessionID)
			if content != c.want {
				t.Errorf("%s: content length %d, want %d (truncated=%v)",
					c.name, len(content), len(c.want), len(content) < len(c.want))
			}
			if strings.HasSuffix(content, "…") || strings.HasSuffix(content, "...") {
				t.Errorf("%s: content must not end with ellipsis", c.name)
			}
			if category != c.wantCat {
				t.Errorf("%s: category = %q, want %q", c.name, category, c.wantCat)
			}
		})
	}
}
