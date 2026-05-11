package notify

import (
	"strings"
	"testing"

	"be/internal/model"
)

func TestRender_SlackVariableSubstitution(t *testing.T) {
	cases := []struct {
		name     string
		template string
		data     map[string]interface{}
		want     string
	}{
		{"event_type", "${event_type}", map[string]interface{}{"event_type": "orchestration.completed"}, "orchestration.completed"},
		{"project_id", "${project_id}", map[string]interface{}{"project_id": "proj-1"}, "proj-1"},
		{"project_name", "${project_name}", map[string]interface{}{"project_name": "My Project"}, "My Project"},
		{"ticket_id", "${ticket_id}", map[string]interface{}{"ticket_id": "T-123"}, "T-123"},
		{"ticket_name", "${ticket_name}", map[string]interface{}{"ticket_name": "Fix the bug"}, "Fix the bug"},
		{"workflow", "${workflow}", map[string]interface{}{"workflow": "feature"}, "feature"},
		{"instance_id", "${instance_id}", map[string]interface{}{"instance_id": "wfi-abc"}, "wfi-abc"},
		{"agent_type", "${agent_type}", map[string]interface{}{"agent_type": "implementor"}, "implementor"},
		{"reason", "${reason}", map[string]interface{}{"reason": "low context"}, "low context"},
		{"multiple", "${event_type}|${workflow}", map[string]interface{}{"event_type": "agent.completed", "workflow": "bugfix"}, "agent.completed|bugfix"},
		{"missing_data", "${event_type}", map[string]interface{}{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(model.ChannelKindSlack, tc.template, tc.data)
			if got != tc.want {
				t.Errorf("Render() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRender_UnknownPlaceholderStripped(t *testing.T) {
	got := Render(model.ChannelKindSlack, "before ${doesNotExist} after", nil)
	want := "before  after"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRender_SlackNoEscaping(t *testing.T) {
	data := map[string]interface{}{"project_name": "my_project", "reason": "low|context"}
	got := Render(model.ChannelKindSlack, "${project_name} ${reason}", data)
	want := "my_project low|context"
	if got != want {
		t.Errorf("Render(slack) = %q, want %q (values must not be escaped)", got, want)
	}
}

func TestRender_TelegramValueEscaping(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		expected string
	}{
		{"underscore", "my_project", `my\_project`},
		{"brackets", "[bug]", `\[bug\]`},
		{"parens", "(note)", `\(note\)`},
		{"dot", "agent.completed", `agent\.completed`},
		{"pipe", "low|context", `low\|context`},
		{"dash", "T-42", `T\-42`},
		{"all_special", `_[]()~>#+-=|{}.!`, `\_\[\]\(\)\~\>\#\+\-\=\|\{\}\.\!`},
		{"no_special", "normaltext", "normaltext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]interface{}{"project_name": tc.value}
			got := Render(model.ChannelKindTelegram, "${project_name}", data)
			if got != tc.expected {
				t.Errorf("Telegram escape %q = %q, want %q", tc.value, got, tc.expected)
			}
		})
	}
}

func TestRender_TelegramTemplateLiteralPassesThrough(t *testing.T) {
	data := map[string]interface{}{"event_type": "orchestration.completed"}
	got := Render(model.ChannelKindTelegram, "*nrflo* — ${event_type}", data)
	want := `*nrflo* — orchestration\.completed`
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRender_LinkSlack(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			"ticket_id",
			map[string]interface{}{"ticket_id": "T-42"},
			"<http://localhost:6587/tickets/T-42|T-42>",
		},
		{
			"instance_id_only",
			map[string]interface{}{"instance_id": "wfi-xyz"},
			"<http://localhost:6587/project-workflows?instance_id=wfi-xyz|project>",
		},
		{"neither", map[string]interface{}{}, ""},
		{
			"ticket_id_takes_precedence",
			map[string]interface{}{"ticket_id": "T-1", "instance_id": "wfi-1"},
			"<http://localhost:6587/tickets/T-1|T-1>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(model.ChannelKindSlack, "${link}", tc.data)
			if got != tc.want {
				t.Errorf("Render(slack, link) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRender_LinkTelegram(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			"ticket_id",
			map[string]interface{}{"ticket_id": "T-42"},
			`[T\-42](http://localhost:6587/tickets/T-42)`,
		},
		{
			"instance_id_only",
			map[string]interface{}{"instance_id": "wfi-xyz"},
			`[project](http://localhost:6587/project-workflows?instance_id=wfi-xyz)`,
		},
		{"neither", map[string]interface{}{}, ""},
		{
			"url_paren_escaped",
			map[string]interface{}{"instance_id": "wfi-)abc"},
			`[project](http://localhost:6587/project-workflows?instance_id=wfi-\)abc)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(model.ChannelKindTelegram, "${link}", tc.data)
			if got != tc.want {
				t.Errorf("Render(telegram, link) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRender_SummaryTruncation(t *testing.T) {
	long := strings.Repeat("a", 1501)
	data := map[string]interface{}{"workflow_final_result": long}
	got := Render(model.ChannelKindSlack, "${summary}", data)
	want := "> " + strings.Repeat("a", 1500) + "…"
	if got != want {
		t.Errorf("summary truncation: got rune-len=%d want rune-len=%d", len([]rune(got)), len([]rune(want)))
	}
}

func TestRender_SummaryShortPassthrough(t *testing.T) {
	data := map[string]interface{}{"workflow_final_result": "Build completed"}
	got := Render(model.ChannelKindSlack, "${summary}", data)
	want := "> Build completed"
	if got != want {
		t.Errorf("Render(summary) = %q, want %q", got, want)
	}
}

func TestRender_SummaryMultilineBlockquote(t *testing.T) {
	data := map[string]interface{}{"workflow_final_result": "line1\nline2\nline3"}
	got := Render(model.ChannelKindSlack, "${summary}", data)
	want := "> line1\n> line2\n> line3"
	if got != want {
		t.Errorf("Render(multiline summary) = %q, want %q", got, want)
	}
}

func TestRender_SummaryEmptyWhenMissing(t *testing.T) {
	data := map[string]interface{}{}
	got := Render(model.ChannelKindSlack, "before ${summary} after", data)
	want := "before  after"
	if got != want {
		t.Errorf("Render(empty summary) = %q, want %q", got, want)
	}
}

func TestRender_TelegramSummaryEscapesContent(t *testing.T) {
	data := map[string]interface{}{"workflow_final_result": "a.b_c"}
	got := Render(model.ChannelKindTelegram, "${summary}", data)
	want := `> a\.b\_c`
	if got != want {
		t.Errorf("Render(telegram summary) = %q, want %q", got, want)
	}
}
