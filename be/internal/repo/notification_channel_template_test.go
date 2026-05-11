package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestNotificationChannelRepo_MessageTemplate_RoundTrip(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	tpl := "custom: ${event_type} — ${workflow}"
	ch := &model.NotificationChannel{
		ProjectID:       projectID,
		WorkflowID:      workflowID,
		Name:            "tpl-channel",
		Kind:            model.ChannelKindSlack,
		Enabled:         true,
		Config:          `{"webhook_url":"https://example.com"}`,
		MessageTemplate: tpl,
		EventTypes:      []string{"orchestration.completed"},
	}
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MessageTemplate != tpl {
		t.Errorf("MessageTemplate = %q, want %q", got.MessageTemplate, tpl)
	}
}

func TestNotificationChannelRepo_MessageTemplate_Update(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p-tpl', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-tpl', 'p-tpl', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	r := NewNotificationChannelRepo(database, clk)

	ch := &model.NotificationChannel{
		ProjectID:       "p-tpl",
		WorkflowID:      "wf-tpl",
		Name:            "ch",
		Kind:            model.ChannelKindSlack,
		Enabled:         true,
		Config:          `{}`,
		MessageTemplate: "original: ${event_type}",
		EventTypes:      []string{},
	}
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	ch.MessageTemplate = "updated: ${workflow}"
	if err := r.Update(ch); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.MessageTemplate != "updated: ${workflow}" {
		t.Errorf("MessageTemplate after update = %q, want %q", got.MessageTemplate, "updated: ${workflow}")
	}
}

func TestNotificationChannelRepo_MessageTemplate_DefaultIsEmpty(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	// Insert without setting MessageTemplate — should default to empty string.
	ch := makeNotifyChannel(projectID, workflowID, "no-tpl", model.ChannelKindSlack, true, nil)
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MessageTemplate != "" {
		t.Errorf("MessageTemplate = %q, want empty string (zero value)", got.MessageTemplate)
	}
}

func TestNotificationChannelRepo_MessageTemplate_TelegramRoundTrip(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	// Must match defaults.go telegramDefaultTemplate.
	tpl := "*nrflo* — ${event_type} ${link}\n${summary}\nagent: ${agent_type} \\| workflow: ${workflow} \\| reason: ${reason} \\| instance: ${instance_id}"
	ch := &model.NotificationChannel{
		ProjectID:       projectID,
		WorkflowID:      workflowID,
		Name:            "tg-tpl",
		Kind:            model.ChannelKindTelegram,
		Enabled:         true,
		Config:          `{"bot_token":"x","chat_id":"1"}`,
		MessageTemplate: tpl,
		EventTypes:      []string{"orchestration.completed"},
	}
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MessageTemplate != tpl {
		t.Errorf("Telegram MessageTemplate round-trip failed\ngot:  %q\nwant: %q", got.MessageTemplate, tpl)
	}
}
