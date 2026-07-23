package handoff

import (
	"context"
	"strings"
	"testing"

	"be/internal/clock"

	"github.com/google/uuid"
)

func TestCompose_AllEmpty_ReturnsEmptyString(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	got := Compose(context.Background(), pool, clock.Real(), sessionID, "")
	if got != "" {
		t.Errorf("Compose() = %q, want empty string when every section is empty", got)
	}
}

func TestCompose_NarrativeOnly_EmitsNarrativeSectionWithPreamble(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")

	got := Compose(context.Background(), pool, clock.Real(), sessionID, "a narrative summary")
	if !strings.Contains(got, "## Narrative Summary") {
		t.Errorf("Compose() = %q, want ## Narrative Summary header", got)
	}
	if !strings.Contains(got, narrativePreamble) {
		t.Errorf("Compose() = %q, want narrative preamble present", got)
	}
	if !strings.Contains(got, "a narrative summary") {
		t.Errorf("Compose() = %q, want narrative text present", got)
	}
	if strings.Contains(got, "## Verified State") {
		t.Errorf("Compose() = %q, want no Verified State section (nothing to verify)", got)
	}
}

func TestCompose_VerifiedOnly_NoNarrativeSection(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "Fix the thing", "")

	got := Compose(context.Background(), pool, clock.Real(), sessionID, "")
	if !strings.Contains(got, "## Verified State") {
		t.Errorf("Compose() = %q, want ## Verified State header (task anchor present)", got)
	}
	if strings.Contains(got, "## Narrative Summary") {
		t.Errorf("Compose() = %q, want no Narrative Summary section when narrative is empty", got)
	}
}

func TestCompose_ThreeSections_TaskPlanTailAndNarrative(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "Fix the thing", "")
	seedFinding(t, pool, wfiID, sessionID, "implementor", "be_files_to_modify", "be/foo.go")
	seedMessages(t, pool, sessionID, []seedMessage{
		{category: "assistant", content: "starting work"},
		{category: "tool", content: "[Bash] make test"},
	})

	got := Compose(context.Background(), pool, clock.Real(), sessionID, "a narrative summary")

	for _, want := range []string{
		"## Verified State",
		"### Task",
		"### Plan (recorded findings)",
		"## Narrative Summary",
		"## Recent Uncompressed Context",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Compose() missing %q; got:\n%s", want, got)
		}
	}
}

func TestCompose_ByteCaps_EnforcedOnRuneBoundaries(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "task", "")

	// A narrative full of multi-byte runes must never be sliced mid-rune.
	hugeNarrative := strings.Repeat("é", maxNarrativeBytes)
	got := Compose(context.Background(), pool, clock.Real(), sessionID, hugeNarrative)

	if len(got) > maxHandoffBytes {
		t.Errorf("Compose() length %d exceeds maxHandoffBytes %d", len(got), maxHandoffBytes)
	}
	if !isValidUTF8Tail(got) {
		t.Errorf("Compose() output is not valid UTF-8 — a cap sliced mid-rune")
	}
}

func TestCompose_UnknownSession_DegradesToEmpty_NoPanic(t *testing.T) {
	pool := newTestPool(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Compose panicked for unknown session: %v", r)
		}
	}()
	got := Compose(context.Background(), pool, clock.Real(), uuid.New().String(), "")
	if got != "" {
		t.Errorf("Compose() = %q, want empty for unknown session", got)
	}
}

func TestCompose_TicketReference_ResolvedInVerifiedState(t *testing.T) {
	pool := newTestPool(t)
	projectID, wfiID, sessionID := newIDs()
	seedProjectAndWorkflow(t, pool, projectID, wfiID, "", "")
	seedSession(t, pool, sessionID, projectID, wfiID, "node-1", "", "")
	seedTicket(t, pool, projectID, "PROJ-123", "Fix the login flow")
	seedMessages(t, pool, sessionID, []seedMessage{
		{category: "assistant", content: "working on PROJ-123 now"},
	})

	got := Compose(context.Background(), pool, clock.Real(), sessionID, "")
	if !strings.Contains(got, "PROJ-123") {
		t.Errorf("Compose() = %q, want ticket ID present", got)
	}
	if !strings.Contains(got, "Fix the login flow") {
		t.Errorf("Compose() = %q, want resolved ticket title present", got)
	}
}

func isValidUTF8Tail(s string) bool {
	for i := 0; i < len(s); {
		r := s[i]
		switch {
		case r < 0x80:
			i++
		case r&0xE0 == 0xC0:
			if i+1 >= len(s) || s[i+1]&0xC0 != 0x80 {
				return false
			}
			i += 2
		case r&0xF0 == 0xE0:
			if i+2 >= len(s) || s[i+1]&0xC0 != 0x80 || s[i+2]&0xC0 != 0x80 {
				return false
			}
			i += 3
		case r&0xF8 == 0xF0:
			if i+3 >= len(s) || s[i+1]&0xC0 != 0x80 || s[i+2]&0xC0 != 0x80 || s[i+3]&0xC0 != 0x80 {
				return false
			}
			i += 4
		default:
			return false
		}
	}
	return true
}
