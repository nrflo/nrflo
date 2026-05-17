package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

func TestNotificationService_Create_Script_EmptyConfig_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	_, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-no-config",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{},
	})
	if err == nil {
		t.Fatalf("Create script with empty config: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "script_code") {
		t.Errorf("error = %q, want to contain 'script_code'", err.Error())
	}
}

func TestNotificationService_Create_Script_EmptyScriptCode_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	_, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-empty-code",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{"script_code": ""},
	})
	if err == nil {
		t.Fatalf("Create script with empty script_code: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "script_code") {
		t.Errorf("error = %q, want to contain 'script_code'", err.Error())
	}
}

func TestNotificationService_Create_Script_InvalidAST_ReturnsError(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; AST check degrades to OK=true")
	}

	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	_, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-invalid-ast",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{"script_code": "def("},
	})
	if err == nil {
		t.Fatalf("Create script with invalid AST: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "syntax") {
		t.Errorf("error = %q, want to contain 'syntax'", err.Error())
	}
}

func TestNotificationService_Create_Script_ValidCode_OK(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-valid",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{"script_code": "print(1)"},
	})
	if err != nil {
		t.Fatalf("Create script with valid code: %v", err)
	}
	if ch.ID == "" {
		t.Errorf("channel ID not set")
	}
	if ch.Kind != "script" {
		t.Errorf("Kind = %q, want script", ch.Kind)
	}
}

func TestNotificationService_Create_Script_ConfigPassthrough_NotMasked(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	code := "import os\nprint(os.environ.get('NRFLO_PROJECT'))"
	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-passthrough",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{"script_code": code},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Script config is not masked — script_code must be verbatim.
	if !strings.Contains(ch.Config, "NRFLO_PROJECT") {
		t.Errorf("Config = %q, want script_code to be present and unmasked", ch.Config)
	}
}

func TestNotificationService_Create_Script_DefaultMessageTemplate_Empty(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "script-tpl",
		Kind:    "script",
		Enabled: &enabled,
		Config:  map[string]interface{}{"script_code": "pass"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MessageTemplate != "" {
		t.Errorf("MessageTemplate = %q, want empty (script channels have no default template)", got.MessageTemplate)
	}
}
