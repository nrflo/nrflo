package notify

import (
	"strings"
	"testing"

	"be/internal/model"
)

func TestDefaultTemplate_SlackMatchesMigrationSQL(t *testing.T) {
	want := "*nrflo* — ${event_type} ${link}\n${summary}\nagent: ${agent_type} | workflow: ${workflow} | reason: ${reason} | instance: ${instance_id}"
	got := DefaultTemplate(model.ChannelKindSlack)
	if got != want {
		t.Errorf("slack default template doesn't match migration SQL\ngot:  %q\nwant: %q", got, want)
	}
}

func TestDefaultTemplate_TelegramMatchesMigrationSQL(t *testing.T) {
	want := "*nrflo* — ${event_type} ${link}\n${summary}\nagent: ${agent_type} \\| workflow: ${workflow} \\| reason: ${reason} \\| instance: ${instance_id}"
	got := DefaultTemplate(model.ChannelKindTelegram)
	if got != want {
		t.Errorf("telegram default template doesn't match migration SQL\ngot:  %q\nwant: %q", got, want)
	}
}

func TestDefaultTemplate_ContainsKeyFields(t *testing.T) {
	cases := []struct {
		kind model.ChannelKind
		name string
	}{
		{model.ChannelKindSlack, "slack"},
		{model.ChannelKindTelegram, "telegram"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := DefaultTemplate(tc.kind)
			if tpl == "" {
				t.Fatalf("DefaultTemplate(%s) returned empty string", tc.name)
			}
			for _, field := range []string{"${event_type}", "${link}"} {
				if !strings.Contains(tpl, field) {
					t.Errorf("%s template missing %s", tc.name, field)
				}
			}
		})
	}
}

func TestDefaultTemplate_SlackRendersWithMinimalData(t *testing.T) {
	tpl := DefaultTemplate(model.ChannelKindSlack)
	data := map[string]interface{}{
		"event_type":  "orchestration.completed",
		"workflow":    "feature",
		"instance_id": "wfi-abc",
	}
	got := Render(model.ChannelKindSlack, tpl, data)
	if !strings.Contains(got, "orchestration.completed") {
		t.Errorf("slack render missing event_type: %q", got)
	}
	if !strings.Contains(got, "wfi-abc") {
		t.Errorf("slack render missing instance_id in link: %q", got)
	}
	if !strings.Contains(got, "feature") {
		t.Errorf("slack render missing workflow: %q", got)
	}
}

func TestDefaultTemplate_TelegramRendersWithMinimalData(t *testing.T) {
	tpl := DefaultTemplate(model.ChannelKindTelegram)
	data := map[string]interface{}{
		"event_type":  "orchestration.completed",
		"workflow":    "feature",
		"instance_id": "wfi-abc",
	}
	got := Render(model.ChannelKindTelegram, tpl, data)
	// Telegram escapes the dot in "orchestration.completed"
	if !strings.Contains(got, `orchestration\.completed`) {
		t.Errorf("telegram render missing escaped event_type: %q", got)
	}
	if !strings.Contains(got, "wfi") {
		t.Errorf("telegram render missing instance ref: %q", got)
	}
}

func TestAvailableVariables_ContainsExpected(t *testing.T) {
	vars := AvailableVariables()
	if len(vars) == 0 {
		t.Fatal("AvailableVariables returned empty slice")
	}
	required := []string{"event_type", "link", "summary", "workflow", "instance_id"}
	for _, r := range required {
		found := false
		for _, v := range vars {
			if v == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AvailableVariables missing %q", r)
		}
	}
}
