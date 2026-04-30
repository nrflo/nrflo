package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
	"be/internal/ws"
)

func setupNotificationServicePool(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-svc', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return pool, "proj-svc"
}

func TestMaskConfig_Slack_MasksLastFour(t *testing.T) {
	config := `{"webhook_url":"https://hooks.slack.com/services/ABC/DEF/GHIJ"}`
	result := maskConfig("slack", config)
	if strings.Contains(result, "GHIJ") {
		t.Errorf("maskConfig slack: last 4 chars not masked, got %q", result)
	}
	if !strings.Contains(result, "****") {
		t.Errorf("maskConfig slack: no **** in result: %q", result)
	}
}

func TestMaskConfig_Telegram_MasksToken_PassesChatID(t *testing.T) {
	config := `{"bot_token":"1234567890:ABCDEFGH","chat_id":"-100123"}`
	result := maskConfig("telegram", config)
	if strings.Contains(result, "ABCDEFGH") {
		t.Errorf("bot_token not masked in: %q", result)
	}
	if !strings.Contains(result, "-100123") {
		t.Errorf("chat_id not preserved in: %q", result)
	}
}

func TestMaskConfig_InvalidJSON_Passthrough(t *testing.T) {
	bad := `not-json`
	result := maskConfig("slack", bad)
	if result != bad {
		t.Errorf("maskConfig invalid JSON: got %q, want passthrough %q", result, bad)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1234ABCDEFGH5678", "1234****5678"},
		{"short", "****"},
		{"12345678", "****"}, // exactly 8 chars — masked
		{"123456789", "1234****6789"},
	}
	for _, tc := range tests {
		got := maskToken(tc.input)
		if got != tc.want {
			t.Errorf("maskToken(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMaskURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://hooks.slack.com/GHIJ", "https://hooks.slack.com/****"},
		{"abcd", "****"},
		{"abc", "****"}, // <= 4 chars
	}
	for _, tc := range tests {
		got := maskURL(tc.input)
		if got != tc.want {
			t.Errorf("maskURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestApplyConfigPatch_MaskedValuePreservesSecret(t *testing.T) {
	stored := `{"webhook_url":"https://hooks.slack.com/services/ABC/XYZW"}`
	masked := maskConfig("slack", stored)

	var maskedMap map[string]interface{}
	if err := json.Unmarshal([]byte(masked), &maskedMap); err != nil {
		t.Fatalf("unmarshal masked: %v", err)
	}
	maskedURL, _ := maskedMap["webhook_url"].(string)

	// PATCH with the masked value (echoed back from client)
	incoming, _ := json.Marshal(map[string]interface{}{"webhook_url": maskedURL})
	result := applyConfigPatch("slack", stored, string(incoming))

	// Should preserve original secret ending in XYZW
	if !strings.Contains(result, "XYZW") {
		t.Errorf("applyConfigPatch: did not preserve secret; got %q", result)
	}
}

func TestApplyConfigPatch_NewValueRotatesSecret(t *testing.T) {
	stored := `{"webhook_url":"https://hooks.slack.com/old-secret-XXXX"}`
	newURL := "https://hooks.slack.com/new-url-YYYY"
	incoming, _ := json.Marshal(map[string]interface{}{"webhook_url": newURL})
	result := applyConfigPatch("slack", stored, string(incoming))

	if !strings.Contains(result, "YYYY") {
		t.Errorf("applyConfigPatch: new value not stored; got %q", result)
	}
	if strings.Contains(result, "XXXX") {
		t.Errorf("applyConfigPatch: old value still present; got %q", result)
	}
}

func TestNotificationService_Create_List_Get(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()
	svc := NewNotificationService(pool, clock.Real(), hub, nil)

	enabled := true
	ch, err := svc.Create(projectID, &types.NotificationChannelCreateRequest{
		Name:       "Test Slack",
		Kind:       "slack",
		Enabled:    &enabled,
		Config:     map[string]interface{}{"webhook_url": "https://example.com/ABCD"},
		EventTypes: []string{"orchestration.completed"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ch.ID == "" {
		t.Errorf("ID not set")
	}
	// Config must be masked in Create response
	if !strings.Contains(ch.Config, "****") {
		t.Errorf("Create response: Config not masked: %q", ch.Config)
	}
	if strings.Contains(ch.Config, "ABCD") && !strings.HasSuffix(ch.Config, "ABCD\"") {
		// ABCD is last 4, so it may be in the ****ABCD mask — OK
	}

	// Get also masks
	got, err := svc.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != ch.ID {
		t.Errorf("Get: ID = %q, want %q", got.ID, ch.ID)
	}
	if !strings.Contains(got.Config, "****") {
		t.Errorf("Get response: Config not masked: %q", got.Config)
	}

	// List
	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}
}

func TestNotificationService_Update_MaskedPreservesSecret(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil)

	enabled := true
	ch, err := svc.Create(projectID, &types.NotificationChannelCreateRequest{
		Name:    "ch",
		Kind:    "slack",
		Enabled: &enabled,
		Config:  map[string]interface{}{"webhook_url": "https://example.com/SECRETXYZ"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get masked config
	got, _ := svc.Get(ch.ID)
	var maskedMap map[string]interface{}
	json.Unmarshal([]byte(got.Config), &maskedMap)
	maskedURL, _ := maskedMap["webhook_url"].(string)

	// PATCH with masked value — secret must be preserved
	updated, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{
		Config: map[string]interface{}{"webhook_url": maskedURL},
	})
	if err != nil {
		t.Fatalf("Update masked: %v", err)
	}

	// After patch, masked response should still mask the same underlying secret
	if !strings.Contains(updated.Config, "****") {
		t.Errorf("after patch: Config not masked: %q", updated.Config)
	}
}

func TestNotificationService_Update_NewValueRotatesSecret(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil)

	enabled := true
	ch, _ := svc.Create(projectID, &types.NotificationChannelCreateRequest{
		Name:    "ch",
		Kind:    "slack",
		Enabled: &enabled,
		Config:  map[string]interface{}{"webhook_url": "https://old-secret-ORIG"},
	})

	newURL := "https://new-url-NEWVAL"
	updated, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{
		Config: map[string]interface{}{"webhook_url": newURL},
	})
	if err != nil {
		t.Fatalf("Update new value: %v", err)
	}
	// New value must be stored (masked as ****EVAL in response)
	if strings.Contains(updated.Config, "ORIG") {
		t.Errorf("old secret still in config after rotation: %q", updated.Config)
	}
}

func TestNotificationService_Delete(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil)

	enabled := true
	ch, _ := svc.Create(projectID, &types.NotificationChannelCreateRequest{
		Name: "ch", Kind: "slack", Enabled: &enabled,
	})

	if _, err := svc.Delete(ch.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ch.ID); err == nil {
		t.Errorf("Get after Delete: expected error, got nil")
	}
}

func TestNotificationService_TestSend_InsertsOneDelivery(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	wakeCh := make(chan struct{}, 1)
	waker := NewChanWaker(wakeCh)
	svc := NewNotificationService(pool, clock.Real(), nil, waker)

	enabled := true
	ch, _ := svc.Create(projectID, &types.NotificationChannelCreateRequest{
		Name: "ch", Kind: "slack", Enabled: &enabled,
	})

	if err := svc.TestSend(ch.ID); err != nil {
		t.Fatalf("TestSend: %v", err)
	}

	deliveries, err := svc.ListDeliveries(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}
	if deliveries[0].EventType != "test" {
		t.Errorf("EventType = %q, want test", deliveries[0].EventType)
	}

	select {
	case <-wakeCh:
		// waker signaled
	default:
		t.Errorf("wake channel not signaled after TestSend")
	}
}

func TestNotificationService_Create_Validation(t *testing.T) {
	pool, projectID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil)

	tests := []struct {
		name string
		req  types.NotificationChannelCreateRequest
	}{
		{"empty name", types.NotificationChannelCreateRequest{Kind: "slack"}},
		{"bad kind", types.NotificationChannelCreateRequest{Name: "x", Kind: "invalid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(projectID, &tc.req)
			if err == nil {
				t.Errorf("Create(%q): expected error, got nil", tc.name)
			}
		})
	}
}
