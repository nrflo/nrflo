package console

import (
	"errors"
	"testing"

	"be/internal/service"
)

func TestChatService_CatalogDiscoversModelsAndLiveSessions(t *testing.T) {
	svc, pool, _, _ := newChatTestService(t)
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("enable API mode: %v", err)
	}
	sid, token, err := svc.CreateAuthenticated("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	catalog, err := svc.Catalog(chatTestProjectID)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog.Engines) != 3 || len(catalog.Sessions) != 1 || catalog.Sessions[0].SessionID != sid {
		t.Fatalf("catalog = %+v", catalog)
	}
	attached, err := svc.AttachAuthenticated(sid, chatTestProjectID)
	if err != nil || attached != token {
		t.Fatalf("AttachAuthenticated token=%q err=%v", attached, err)
	}
}

func TestChatService_AttachAuthenticatedRejectsProjectMismatch(t *testing.T) {
	svc, _, _, _ := newChatTestService(t)
	sid, _, err := svc.CreateAuthenticated("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	if _, err := svc.AttachAuthenticated(sid, "another-project"); !errors.Is(err, ErrChatProjectMismatch) {
		t.Fatalf("AttachAuthenticated error = %v", err)
	}
}
