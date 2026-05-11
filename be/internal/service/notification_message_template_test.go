package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/notify"
	"be/internal/types"
)

func TestNotificationService_Create_DefaultMessageTemplate_Slack(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, err := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "slack-default",
		Kind:    "slack",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Fetch the raw channel to check the stored MessageTemplate (not masked).
	raw, err := svc.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := notify.DefaultTemplate(model.ChannelKindSlack)
	if raw.MessageTemplate != want {
		t.Errorf("MessageTemplate = %q, want slack default %q", raw.MessageTemplate, want)
	}
}

func TestNotificationService_Create_DefaultMessageTemplate_Telegram(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, err := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "tg-default",
		Kind:    "telegram",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw, err := svc.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := notify.DefaultTemplate(model.ChannelKindTelegram)
	if raw.MessageTemplate != want {
		t.Errorf("MessageTemplate = %q, want telegram default %q", raw.MessageTemplate, want)
	}
}

func TestNotificationService_Create_ExplicitMessageTemplate(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	custom := "my custom ${event_type}"
	enabled := true
	ch, err := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:            "explicit-tpl",
		Kind:            "slack",
		Enabled:         &enabled,
		MessageTemplate: &custom,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw, err := svc.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if raw.MessageTemplate != custom {
		t.Errorf("MessageTemplate = %q, want %q", raw.MessageTemplate, custom)
	}
}

func TestNotificationService_Update_MessageTemplate_NilNoChange(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	custom := "keep me"
	enabled := true
	ch, _ := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:            "keep-tpl",
		Kind:            "slack",
		Enabled:         &enabled,
		MessageTemplate: &custom,
	})

	// Update with nil MessageTemplate → no change.
	_, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	raw, _ := svc.Get(ch.ID)
	if raw.MessageTemplate != custom {
		t.Errorf("MessageTemplate after nil update = %q, want %q (should not change)", raw.MessageTemplate, custom)
	}
}

func TestNotificationService_Update_MessageTemplate_EmptyResetsToDefault(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	custom := "some custom template"
	enabled := true
	ch, _ := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:            "reset-tpl",
		Kind:            "slack",
		Enabled:         &enabled,
		MessageTemplate: &custom,
	})

	// Update with *"" → reset to default.
	empty := ""
	_, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{
		MessageTemplate: &empty,
	})
	if err != nil {
		t.Fatalf("Update to empty: %v", err)
	}

	raw, _ := svc.Get(ch.ID)
	want := notify.DefaultTemplate(model.ChannelKindSlack)
	if raw.MessageTemplate != want {
		t.Errorf("MessageTemplate after empty update = %q, want default %q", raw.MessageTemplate, want)
	}
}

func TestNotificationService_Update_MessageTemplate_ExplicitSets(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, _ := svc.Create(projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "set-tpl",
		Kind:    "slack",
		Enabled: &enabled,
	})

	newTpl := "new: ${event_type} ${workflow}"
	_, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{
		MessageTemplate: &newTpl,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	raw, _ := svc.Get(ch.ID)
	if raw.MessageTemplate != newTpl {
		t.Errorf("MessageTemplate after update = %q, want %q", raw.MessageTemplate, newTpl)
	}
}
