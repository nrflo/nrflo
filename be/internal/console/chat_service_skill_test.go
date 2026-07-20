package console

import (
	"errors"
	"strings"
	"testing"

	"be/internal/service"
)

// TestChatService_ListSkills_UnknownProject_ReturnsSentinel mirrors Catalog's
// existence check.
func TestChatService_ListSkills_UnknownProject_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	_, err := svc.ListSkills("no-such-project")
	if !errors.Is(err, service.ErrConsoleProjectNotFound) {
		t.Errorf("ListSkills(unknown) = %v, want service.ErrConsoleProjectNotFound", err)
	}
}

// TestChatService_ListSkills_NoRootPath_ReturnsEmptyNoError covers a known
// project with an empty/invalid root_path (chatTestProjectID is seeded with
// no root_path in newChatTestService).
func TestChatService_ListSkills_NoRootPath_ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	got, err := svc.ListSkills(chatTestProjectID)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSkills(no root_path) = %+v, want empty", got)
	}
}

// TestChatService_ListSkills_DiscoversProjectSkills sets a real root_path and
// asserts the discovered skill surfaces through the service.
func TestChatService_ListSkills_DiscoversProjectSkills(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	root := buildSkillTree(t)
	if _, err := pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, root, chatTestProjectID); err != nil {
		t.Fatalf("set root_path: %v", err)
	}

	got, err := svc.ListSkills(chatTestProjectID)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListSkills = %+v, want 3 skills", got)
	}
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "finalize") {
		t.Errorf("names = %v, want to include finalize", names)
	}
}

// TestChatService_SendMessage_SkillDispatch is the ChatService-facing
// dispatch test from the plan: a matched skill rides on UserTurn.Skill with
// turn.Text left as the ORIGINAL "/name args" text; an unknown "/name"
// carries no skill and passes text verbatim; plain text is unaffected.
func TestChatService_SendMessage_SkillDispatch(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	root := buildSkillTree(t)
	if _, err := pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, root, chatTestProjectID); err != nil {
		t.Fatalf("set root_path: %v", err)
	}

	t.Run("matched skill", func(t *testing.T) {
		sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		eng := factory.last()

		if err := svc.SendMessage(sid, "/finalize extra args"); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if got := eng.turnCount(); got != 1 {
			t.Fatalf("turnCount = %d, want 1", got)
		}
		if got := eng.turns[0]; got != "/finalize extra args" {
			t.Errorf("turn text = %q, want the original raw text", got)
		}
		match := eng.skills[0]
		if match == nil {
			t.Fatal("turn.Skill = nil, want a resolved SkillMatch")
		}
		if match.Name != "finalize" || match.Args != "extra args" || match.Body != "FINALIZE BODY" {
			t.Errorf("match = %+v, want Name=finalize Args=%q Body=%q", match, "extra args", "FINALIZE BODY")
		}
	})

	t.Run("unknown skill sent verbatim", func(t *testing.T) {
		sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		eng := factory.last()

		if err := svc.SendMessage(sid, "/unknown"); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if got := eng.turns[0]; got != "/unknown" {
			t.Errorf("turn text = %q, want verbatim %q", got, "/unknown")
		}
		if eng.skills[0] != nil {
			t.Errorf("turn.Skill = %+v, want nil for an unknown skill name", eng.skills[0])
		}
	})

	t.Run("plain text unaffected", func(t *testing.T) {
		sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		eng := factory.last()

		if err := svc.SendMessage(sid, "plain question"); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if got := eng.turns[0]; got != "plain question" {
			t.Errorf("turn text = %q, want verbatim %q", got, "plain question")
		}
		if eng.skills[0] != nil {
			t.Errorf("turn.Skill = %+v, want nil for plain text", eng.skills[0])
		}
	})
}

// TestChatService_SendMessage_SkillTurn_DefersSeedContext covers the
// seed-deferral acceptance case: a sibling chat with a pending seedContext
// whose first message is a matched skill must NOT apply the seed prepend —
// turn.Text stays the raw "/name args" text (the skill's own body is the
// turn's context, not the digest prepend).
func TestChatService_SendMessage_SkillTurn_DefersSeedContext(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	root := buildSkillTree(t)
	if _, err := pool.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, root, chatTestProjectID); err != nil {
		t.Fatalf("set root_path: %v", err)
	}

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatalf("session %s not found", sid)
	}
	sess.setSeedContext("SEED-DIGEST-XYZ")
	eng := factory.last()

	if err := svc.SendMessage(sid, "/finalize"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := eng.turns[0]; got != "/finalize" {
		t.Errorf("turn text = %q, want the raw skill text with NO seed prefix", got)
	}
	if strings.Contains(eng.turns[0], "SEED-DIGEST-XYZ") {
		t.Errorf("turn text = %q, must not contain the deferred seed digest", eng.turns[0])
	}
	if eng.skills[0] == nil || eng.skills[0].Name != "finalize" {
		t.Errorf("turn.Skill = %+v, want a resolved finalize match", eng.skills[0])
	}
	// The seed is deferred (unconsumed this turn), not dropped — still pending
	// on the session for whatever consumes it next.
	if got := sess.takeSeedContext(); got != "SEED-DIGEST-XYZ" {
		t.Errorf("seedContext after skill turn = %q, want still pending %q", got, "SEED-DIGEST-XYZ")
	}
}
