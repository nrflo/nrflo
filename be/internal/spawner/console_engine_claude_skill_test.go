package spawner

import (
	"context"
	"testing"
)

// TestClaudeEngine_SendUserTurn_SkillIgnored_WritesRawTextWithPaletteSpace is
// the claude side of the Rule-6 dispatch acceptance test: turn.Skill is
// ignored entirely — the raw "/name args" text is typed into the PTY
// unchanged, with the claude-only trailing-space palette mitigation (a
// leading '/' opens the command-palette autocomplete, so a space is written
// before the submit CR to dismiss it) and the un-spaced text still submitted.
func TestClaudeEngine_SendUserTurn_SkillIgnored_WritesRawTextWithPaletteSpace(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.NotifySessionReady()

	skill := &SkillMatch{Name: "foo", Body: "BODY THAT MUST NOT APPEAR", Args: "x"}
	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "/foo x", Skill: skill}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	sess := mgr.sessions[e.spec.SessionID]
	sess.mu.Lock()
	written := string(sess.writtenBytes)
	sess.mu.Unlock()

	// Raw text + palette-dismissing space + submit CR — never the expanded
	// skill body (that's codex/api's job, not claude's).
	if want := "/foo x \r"; written != want {
		t.Errorf("PTY bytes = %q, want %q (raw text unchanged by the skill match)", written, want)
	}

	if n := countCategory(sink, "user_input"); n != 1 {
		t.Fatalf("user_input rows = %d, want 1", n)
	}
	var found bool
	for _, m := range sink.recordedMsgs {
		if m.category == "user_input" && m.content == "/foo x" {
			found = true
		}
	}
	if !found {
		t.Errorf("no user_input row with the raw text %q: %+v", "/foo x", sink.recordedMsgs)
	}
}

// TestClaudeEngine_SendUserTurn_LeadingSlash_PendingEchoStaysUnspaced asserts
// the palette-dismissing trailing space is isolated to the PTY write: the
// dedupe echo armed for NotifyUserPrompt still matches the un-spaced raw
// text, so a human's identical hook-reported prompt is still recognized as
// this engine's own submission (own=true).
func TestClaudeEngine_SendUserTurn_LeadingSlash_PendingEchoStaysUnspaced(t *testing.T) {
	sink := &testSink{}
	e, _ := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.NotifySessionReady()

	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "/finalize"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	if own := e.NotifyUserPrompt("/finalize"); !own {
		t.Errorf("NotifyUserPrompt(%q) = %v, want true (own submission, un-spaced)", "/finalize", own)
	}
	// Consumed once: a second identical prompt is no longer recognized as our
	// own echo.
	if own := e.NotifyUserPrompt("/finalize"); own {
		t.Errorf("second NotifyUserPrompt(%q) = %v, want false (dedupe consumed once)", "/finalize", own)
	}
}

// TestClaudeEngine_SendUserTurn_NonSlashText_NoTrailingSpace asserts the
// palette mitigation only fires for leading '/' text — a plain message is
// written with no injected space.
func TestClaudeEngine_SendUserTurn_NonSlashText_NoTrailingSpace(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.NotifySessionReady()

	if err := e.SendUserTurn(context.Background(), UserTurn{Text: "plain message"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	sess := mgr.sessions[e.spec.SessionID]
	if got := string(sess.writtenBytes); got != "plain message\r" {
		t.Errorf("PTY bytes = %q, want %q (no palette-dismissing space for non-slash text)", got, "plain message\r")
	}
}
