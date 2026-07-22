package refinery

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// stubBuildProvider swaps the buildProvider test seam for the duration of the
// test, restoring the original on cleanup, so fold never needs real provider
// credentials or a network call.
func stubBuildProvider(t *testing.T, prov provider.Provider) {
	t.Helper()
	orig := buildProvider
	buildProvider = func(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) {
		return prov, nil
	}
	t.Cleanup(func() { buildProvider = orig })
}

func TestFold_UpsertsDigestAndIncrementsVersionAndFoldCount(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-fold-1", "proj-fold-1"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(
		mockScript("digest v1"),
		mockScript("digest v2"),
	))

	mgr.fold(context.Background(), sessionID, projectID, []string{`{"type":"findings.updated"}`})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get after first fold: %v", err)
	}
	if d == nil {
		t.Fatal("Get after first fold = nil, want a digest row")
	}
	if d.Content != "digest v1" {
		t.Errorf("Content = %q, want %q", d.Content, "digest v1")
	}
	if d.Version != 1 {
		t.Errorf("Version = %d, want 1", d.Version)
	}
	if d.FoldCount != 1 {
		t.Errorf("FoldCount = %d, want 1", d.FoldCount)
	}

	mgr.fold(context.Background(), sessionID, projectID, []string{`{"type":"orchestration.completed"}`})

	d2, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get after second fold: %v", err)
	}
	if d2.Content != "digest v2" {
		t.Errorf("second fold Content = %q, want %q", d2.Content, "digest v2")
	}
	if d2.Version != 2 {
		t.Errorf("second fold Version = %d, want 2 (bumped)", d2.Version)
	}
	if d2.FoldCount != 2 {
		t.Errorf("second fold FoldCount = %d, want 2 (bumped)", d2.FoldCount)
	}

	var rowCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM refinery_digests WHERE console_session_id = ?`, sessionID).Scan(&rowCount); err != nil {
		t.Fatalf("count refinery_digests rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("refinery_digests row count for session = %d, want exactly 1 (single-row upsert)", rowCount)
	}
}

func TestFold_TruncatesOverCapDigestOnUTF8Boundary(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-fold-trunc", "proj-fold-trunc"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	// 4100 'a' bytes then a 3-byte € straddling the 4096 cap, so truncation
	// must back off to the UTF-8 rune boundary rather than split it.
	oversized := strings.Repeat("a", maxDigestBytes-2) + "€" + "TAIL"
	stubBuildProvider(t, mock.New(mockScript(oversized)))

	mgr.fold(context.Background(), sessionID, projectID, []string{"event"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d == nil {
		t.Fatal("Get after fold = nil")
	}
	if len(d.Content) > maxDigestBytes {
		t.Errorf("Content length = %d, want <= %d", len(d.Content), maxDigestBytes)
	}
	if strings.Contains(d.Content, "TAIL") {
		t.Error("truncated content should not contain bytes past the 4KB cap")
	}
	if !strings.HasPrefix(oversized, d.Content) {
		t.Error("truncated content is not a valid prefix of the original response")
	}
}

func TestFold_MissingRefineryDef_SkipsWithoutPanicking(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-fold-nodef", "proj-fold-nodef"
	seedConsoleChatSession(t, pool, sessionID, projectID)
	if _, err := pool.Exec(`DELETE FROM system_agent_definitions WHERE id = '_refinery'`); err != nil {
		t.Fatalf("delete _refinery def: %v", err)
	}

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("unused")))

	mgr.fold(context.Background(), sessionID, projectID, []string{"event"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d != nil {
		t.Errorf("Get after fold with no _refinery def = %+v, want nil (fold should have skipped)", d)
	}
}

func TestBuildFoldUserText_TaskAnchor(t *testing.T) {
	t.Run("non-empty anchor renders Task section with labeled event lines", func(t *testing.T) {
		got := buildFoldUserText("Implement the widget.", "prev digest", []string{"[user_input] please add a widget", "[tool] ran ls"})
		if !strings.Contains(got, "## Task\n\nImplement the widget.") {
			t.Errorf("buildFoldUserText = %q, want a ## Task section with the anchor verbatim", got)
		}
		if !strings.Contains(got, "[user_input] please add a widget") {
			t.Errorf("buildFoldUserText = %q, want the [user_input] labeled line", got)
		}
		if !strings.Contains(got, "[tool] ran ls") {
			t.Errorf("buildFoldUserText = %q, want the [tool] labeled line", got)
		}
	})

	t.Run("empty anchor omits Task section (console-fold parity)", func(t *testing.T) {
		got := buildFoldUserText("", "prev digest", []string{"event one"})
		if strings.Contains(got, "## Task") {
			t.Errorf("buildFoldUserText with empty anchor = %q, want no ## Task section", got)
		}
		if !strings.Contains(got, "## Previous Digest") {
			t.Errorf("buildFoldUserText = %q, want the Previous Digest section still present", got)
		}
	})
}

func mockScript(text string) mock.Script {
	return mock.Script{Final: provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: text}},
	}}
}
