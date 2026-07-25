package console

import (
	"errors"
	"testing"

	"be/internal/service"
	"be/internal/types"
)

func TestChatService_CatalogDiscoversModelsAndLiveSessions(t *testing.T) {
	svc, pool, _, _ := newChatTestService(t)
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("enable API mode: %v", err)
	}
	sid, token, err := svc.CreateAuthenticated("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	// Newest-release models sort first; bump one row's release_date so it
	// wins ahead of the migration-seeded ordering.
	mustExec(t, pool, `UPDATE models SET release_date = '2099-01-01' WHERE id = 'haiku-4-5'`)
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
	sid, _, err := svc.CreateAuthenticated("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	if _, err := svc.AttachAuthenticated(sid, "another-project"); !errors.Is(err, ErrChatProjectMismatch) {
		t.Fatalf("AttachAuthenticated error = %v", err)
	}
}

// TestChatService_Catalog_ListsBuiltinProfiles verifies the catalog's
// Profiles field surfaces both t0-decider and t0-hands with the wire-shape
// fields a chat-creation picker needs, sourced from ListProfiles().
func TestChatService_Catalog_ListsBuiltinProfiles(t *testing.T) {
	svc, _, _, _ := newChatTestService(t)
	catalog, err := svc.Catalog(chatTestProjectID)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	byName := map[string]types.ConsoleProfileOption{}
	for _, p := range catalog.Profiles {
		byName[p.Name] = p
	}
	decider, ok := byName["t0-decider"]
	if !ok {
		t.Fatal("catalog.Profiles missing t0-decider")
	}
	if decider.DefaultEngine != "claude" || decider.DefaultModelID != "opus-5" || decider.ContextBudgetTokens != 50000 || !decider.RefineryDefault {
		t.Errorf("t0-decider profile option = %+v, want claude/opus-5/50000/refinery-on", decider)
	}
	hands, ok := byName["t0-hands"]
	if !ok {
		t.Fatal("catalog.Profiles missing t0-hands")
	}
	if hands.DefaultModelID != "sonnet-5" || hands.ContextBudgetTokens != 150000 || !hands.RefineryDefault {
		t.Errorf("t0-hands profile option = %+v, want sonnet-5/150000/refinery-on", hands)
	}
}

// TestChatService_Catalog_SessionsCarryProfileName verifies a live chat
// created under a named profile surfaces that name on its
// ConsoleSessionOption, and a plain (no-profile) chat surfaces "".
func TestChatService_Catalog_SessionsCarryProfileName(t *testing.T) {
	svc, _, _, _ := newChatTestService(t)
	deciderSID, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	plainSID, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create(plain): %v", err)
	}
	catalog, err := svc.Catalog(chatTestProjectID)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	byID := map[string]string{}
	for _, s := range catalog.Sessions {
		byID[s.SessionID] = s.Profile
	}
	if got := byID[deciderSID]; got != "t0-decider" {
		t.Errorf("session %q Profile = %q, want t0-decider", deciderSID, got)
	}
	if got, ok := byID[plainSID]; !ok || got != "" {
		t.Errorf("session %q Profile = %q, want empty string", plainSID, got)
	}
}
