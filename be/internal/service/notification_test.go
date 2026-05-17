package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/notify"
	"be/internal/repo"
	"be/internal/types"
	"be/internal/ws"
)

// setupNotificationServicePool creates an in-memory DB with a seeded project + workflow.
func setupNotificationServicePool(t *testing.T) (*db.Pool, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-svc', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err = pool.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-svc', 'proj-svc', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	return pool, "proj-svc", "wf-svc"
}

func TestNotificationService_Create_List_Get(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()
	svc := NewNotificationService(pool, clock.Real(), hub, nil, nil)

	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
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
	if !strings.Contains(ch.Config, "****") {
		t.Errorf("Create response: Config not masked: %q", ch.Config)
	}

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

	list, err := svc.List(projectID, workflowID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}
}

func TestNotificationService_Update_MaskedPreservesSecret(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name:    "ch",
		Kind:    "slack",
		Enabled: &enabled,
		Config:  map[string]interface{}{"webhook_url": "https://example.com/SECRETXYZ"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := svc.Get(ch.ID)
	var maskedMap map[string]interface{}
	json.Unmarshal([]byte(got.Config), &maskedMap)
	maskedURL, _ := maskedMap["webhook_url"].(string)

	updated, err := svc.Update(ch.ID, &types.NotificationChannelUpdateRequest{
		Config: map[string]interface{}{"webhook_url": maskedURL},
	})
	if err != nil {
		t.Fatalf("Update masked: %v", err)
	}
	if !strings.Contains(updated.Config, "****") {
		t.Errorf("after patch: Config not masked: %q", updated.Config)
	}
}

func TestNotificationService_Update_NewValueRotatesSecret(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, _ := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
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
	if strings.Contains(updated.Config, "ORIG") {
		t.Errorf("old secret still in config after rotation: %q", updated.Config)
	}
}

func TestNotificationService_Delete(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	enabled := true
	ch, _ := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
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
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	wakeCh := make(chan struct{}, 1)
	waker := NewChanWaker(wakeCh)
	svc := NewNotificationService(pool, clock.Real(), nil, waker, nil)

	enabled := true
	ch, _ := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
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
	default:
		t.Errorf("wake channel not signaled after TestSend")
	}
}

func TestNotificationService_Create_Validation(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	svc := NewNotificationService(pool, clock.Real(), nil, nil, nil)

	tests := []struct {
		name string
		req  types.NotificationChannelCreateRequest
	}{
		{"empty name", types.NotificationChannelCreateRequest{Kind: "slack"}},
		{"bad kind", types.NotificationChannelCreateRequest{Name: "x", Kind: "invalid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), projectID, workflowID, &tc.req)
			if err == nil {
				t.Errorf("Create(%q): expected error, got nil", tc.name)
			}
		})
	}
}

func TestNotificationService_TestSend_PayloadCoversAllVariables(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupNotificationServicePool(t)
	clk := clock.Real()
	svc := NewNotificationService(pool, clk, nil, nil, nil)

	enabled := true
	ch, err := svc.Create(context.Background(), projectID, workflowID, &types.NotificationChannelCreateRequest{
		Name: "payload-vars", Kind: "slack", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.TestSend(ch.ID); err != nil {
		t.Fatalf("TestSend: %v", err)
	}

	dr := repo.NewNotificationDeliveryRepo(pool, clk)
	deliveries, err := dr.ListByChannel(ch.ID, 50)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(deliveries[0].Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// "link" and "summary" are render-time computed from ticket_id and workflow_final_result;
	// all other AvailableVariables() keys must be direct non-empty payload fields.
	for _, key := range notify.AvailableVariables() {
		if key == "link" || key == "summary" {
			continue
		}
		val, ok := payload[key]
		if !ok {
			t.Errorf("payload missing key %q", key)
			continue
		}
		s, isStr := val.(string)
		if !isStr || s == "" {
			t.Errorf("payload[%q] = %v, want non-empty string", key, val)
		}
	}

	// Rendering the default template with this payload should resolve all placeholders.
	rendered := notify.Render(model.ChannelKindSlack, notify.DefaultTemplate(model.ChannelKindSlack), payload)
	if strings.Contains(rendered, "${") {
		t.Errorf("rendered template has unresolved placeholders: %q", rendered)
	}
	if rendered == "" {
		t.Errorf("rendered template is empty")
	}
}

func TestNotificationService_Create_UnknownWorkflow_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID, _ := setupNotificationServicePool(t)
	wfSvc := NewWorkflowService(pool, clock.Real())
	svc := NewNotificationService(pool, clock.Real(), nil, nil, wfSvc)

	enabled := true
	_, err := svc.Create(context.Background(), projectID, "no-such-workflow", &types.NotificationChannelCreateRequest{
		Name: "ch", Kind: "slack", Enabled: &enabled,
	})
	if err == nil {
		t.Fatalf("Create with unknown workflow: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}
