package console

import (
	"testing"

	"be/internal/spawner"
)

// A server-authored wake-up lands as spawner.CategorySystemTurn, so the TUI
// renders it as a muted notice instead of blue human input and the composer's
// input-history recall (which filters on user_input) never offers it back.
func TestChatService_SendNotification_PersistsAsSystemTurn(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SendNotification(sid, "[nrflo] Delegation abc finished"); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	cats := factory.last().turnCategoryTexts()
	if len(cats) != 1 || cats[0] != spawner.CategorySystemTurn {
		t.Errorf("turn categories = %v, want [%s]", cats, spawner.CategorySystemTurn)
	}
}

// A human message keeps the default category — the notification path must not
// leak its category onto ordinary input.
func TestChatService_SendMessage_PersistsAsUserInput(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SendMessage(sid, "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	cats := factory.last().turnCategoryTexts()
	if len(cats) != 1 || cats[0] != spawner.CategoryUserInput {
		t.Errorf("turn categories = %v, want [%s]", cats, spawner.CategoryUserInput)
	}
}

// A notification steered into a running turn carries its category too — the
// steer path persists its own row.
func TestChatService_SendNotification_MidTurn_SteersAsSystemTurn(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SendMessage(sid, "first"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if _, err := svc.SendNotification(sid, "[nrflo] Delegation abc finished"); err != nil {
		t.Fatalf("SendNotification mid-turn: %v", err)
	}
	cats := factory.last().steerCategoryTexts()
	if len(cats) != 1 || cats[0] != spawner.CategorySystemTurn {
		t.Errorf("steer categories = %v, want [%s]", cats, spawner.CategorySystemTurn)
	}
}

// mergeCategory's rule: a turn composed from both human and server text is
// attributed to the human, since one row cannot carry two authors and
// under-claiming server authorship is the safe direction.
func TestMergeCategory(t *testing.T) {
	t.Parallel()
	sys := queuedPrompt{text: "n", category: spawner.CategorySystemTurn}
	human := queuedPrompt{text: "h", category: spawner.CategoryUserInput}

	tests := []struct {
		name     string
		queued   []queuedPrompt
		incoming string
		want     string
	}{
		{"all server", []queuedPrompt{sys, sys}, spawner.CategorySystemTurn, spawner.CategorySystemTurn},
		{"queued human, server incoming", []queuedPrompt{sys, human}, spawner.CategorySystemTurn, spawner.CategoryUserInput},
		{"queued server, human incoming", []queuedPrompt{sys}, spawner.CategoryUserInput, spawner.CategoryUserInput},
		{"empty queue", nil, spawner.CategorySystemTurn, spawner.CategorySystemTurn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeCategory(tt.queued, tt.incoming); got != tt.want {
				t.Errorf("mergeCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}
