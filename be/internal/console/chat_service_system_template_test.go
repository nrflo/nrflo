package console

// Tests for ChatService.Create/CreateAuthenticated's systemTemplateID param:
// it flows through chatSpecParams.SystemTemplateID into buildChatEngineSpec,
// which renders it into EngineSpec.SystemPrompt before Start is called.

import (
	"strings"
	"testing"
)

// TestChatService_Create_SystemTemplateID_ReachesEngineSpec verifies a
// non-empty systemTemplateID param flows through chatSpecParams into
// EngineSpec.SystemPrompt, rendered against the migrated tier-t2-extractor
// injectable.
func TestChatService_Create_SystemTemplateID_ReachesEngineSpec(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "tier-t2-extractor")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sid == "" {
		t.Fatal("Create returned empty session id")
	}

	eng := factory.last()
	if eng == nil {
		t.Fatal("no fake engine constructed")
	}
	spec := eng.spec()
	if spec.SystemPrompt == "" {
		t.Fatal("spec.SystemPrompt is empty, want the rendered tier-t2-extractor template")
	}
	if !strings.Contains(spec.SystemPrompt, "T2 Extractor") {
		t.Errorf("spec.SystemPrompt = %q, want it to contain the tier-t2-extractor role text", spec.SystemPrompt)
	}
}

// TestChatService_Create_EmptySystemTemplateID_LeavesSpecSystemPromptEmpty is
// the byte-identical regression: an unset systemTemplateID must leave
// EngineSpec.SystemPrompt empty, exactly as before this feature.
func TestChatService_Create_EmptySystemTemplateID_LeavesSpecSystemPromptEmpty(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sid == "" {
		t.Fatal("Create returned empty session id")
	}

	eng := factory.last()
	if eng == nil {
		t.Fatal("no fake engine constructed")
	}
	if spec := eng.spec(); spec.SystemPrompt != "" {
		t.Errorf("spec.SystemPrompt = %q, want empty when systemTemplateID is unset", spec.SystemPrompt)
	}
}

// TestChatService_CreateAuthenticated_SystemTemplateID_ReachesEngineSpec
// mirrors the HTTP-path test above for the socket-facing CreateAuthenticated
// entry point.
func TestChatService_CreateAuthenticated_SystemTemplateID_ReachesEngineSpec(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, _, err := svc.CreateAuthenticated("codex", "", "", chatTestProjectID, "tier-t1-executor")
	if err != nil {
		t.Fatalf("CreateAuthenticated: %v", err)
	}
	if sid == "" {
		t.Fatal("CreateAuthenticated returned empty session id")
	}

	eng := factory.last()
	if eng == nil {
		t.Fatal("no fake engine constructed")
	}
	spec := eng.spec()
	if !strings.Contains(spec.SystemPrompt, "T1 Executor") {
		t.Errorf("spec.SystemPrompt = %q, want it to contain the tier-t1-executor role text", spec.SystemPrompt)
	}
}
