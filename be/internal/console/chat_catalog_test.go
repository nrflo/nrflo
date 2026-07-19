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
	sid, token, err := svc.CreateAuthenticated("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	// Newest-added models sort first; seeded rows share a timestamp, so bump one.
	mustExec(t, pool, `UPDATE models SET created_at = '2099-01-01T00:00:00Z' WHERE id = 'haiku-4-5'`)
	catalog, err := svc.Catalog(chatTestProjectID)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog.Engines) != 3 || len(catalog.Sessions) != 1 || catalog.Sessions[0].SessionID != sid {
		t.Fatalf("catalog = %+v", catalog)
	}
	claude, api := catalog.Engines[0], catalog.Engines[2]
	if claude.Kind != "cli" || claude.Brand != "claude" {
		t.Fatalf("claude engine kind/brand = %q/%q", claude.Kind, claude.Brand)
	}
	if api.Kind != "api" || api.Brand != "" {
		t.Fatalf("api engine kind/brand = %q/%q", api.Kind, api.Brand)
	}
	if len(api.Models) == 0 || api.Models[0].ID != "haiku-4-5" || api.Models[0].Brand != "claude" {
		t.Fatalf("api models[0] = %+v, want newest-added haiku-4-5 with brand claude", api.Models)
	}
	if api.Models[0].MappedModel != "claude-haiku-4-5" || len(api.Models[0].SupportedEfforts) != 3 {
		t.Fatalf("api haiku mode fields = %+v", api.Models[0])
	}
	for _, m := range api.Models {
		if m.Provider == "openai" && m.Brand != "gpt" {
			t.Fatalf("openai model %q brand = %q, want gpt", m.ID, m.Brand)
		}
	}
	attached, err := svc.AttachAuthenticated(sid, chatTestProjectID)
	if err != nil || attached != token {
		t.Fatalf("AttachAuthenticated token=%q err=%v", attached, err)
	}
}

func TestChatService_AttachAuthenticatedRejectsProjectMismatch(t *testing.T) {
	svc, _, _, _ := newChatTestService(t)
	sid, _, err := svc.CreateAuthenticated("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	if _, err := svc.AttachAuthenticated(sid, "another-project"); !errors.Is(err, ErrChatProjectMismatch) {
		t.Fatalf("AttachAuthenticated error = %v", err)
	}
}
