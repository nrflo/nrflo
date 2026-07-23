package repo

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestRefineryRun_InsertAndListRecent(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRefineryRunRepo(pool, clk)

	okRun := &model.RefineryRun{
		SessionID:    "sess-ok",
		ProjectID:    "proj",
		Provider:     "anthropic",
		Model:        "haiku-4-5",
		PromptTokens: 100,
		OutputTokens: 20,
		Status:       "ok",
		FoldCount:    1,
		FoldedAt:     clk.Now().Add(time.Minute),
	}
	if err := r.Insert(okRun); err != nil {
		t.Fatalf("Insert ok run: %v", err)
	}

	failedRun := &model.RefineryRun{
		SessionID: "sess-failed",
		ProjectID: "proj",
		Provider:  "anthropic",
		Model:     "haiku-4-5",
		Status:    "failed",
		Error:     "no anthropic API key",
		FoldedAt:  clk.Now().Add(2 * time.Minute),
	}
	if err := r.Insert(failedRun); err != nil {
		t.Fatalf("Insert failed run: %v", err)
	}

	got, err := r.ListRecent(50, time.Time{})
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	// Newest first: failedRun (folded 2 minutes in) precedes okRun (1 minute).
	if got[0].SessionID != "sess-failed" || got[1].SessionID != "sess-ok" {
		t.Errorf("got order = [%s, %s], want [sess-failed, sess-ok] (folded_at DESC)", got[0].SessionID, got[1].SessionID)
	}
	if got[1].Status != "ok" || got[1].PromptTokens != 100 || got[1].OutputTokens != 20 {
		t.Errorf("ok row = %+v, want status=ok tokens 100/20", got[1])
	}
	if got[0].Status != "failed" || !strings.Contains(got[0].Error, "no anthropic API key") {
		t.Errorf("failed row = %+v, want status=failed error containing %q", got[0], "no anthropic API key")
	}
}

func TestRefineryRun_ListRecent_LimitAndSince(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRefineryRunRepo(pool, clk)

	base := clk.Now()
	for i := 0; i < 4; i++ {
		run := &model.RefineryRun{
			SessionID: "sess-" + string(rune('a'+i)),
			ProjectID: "proj",
			Provider:  "anthropic",
			Model:     "haiku-4-5",
			Status:    "ok",
			FoldedAt:  base.Add(time.Duration(i) * time.Minute),
		}
		if err := r.Insert(run); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}

	limited, err := r.ListRecent(2, time.Time{})
	if err != nil {
		t.Fatalf("ListRecent limit: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("len(limited) = %d, want 2", len(limited))
	}
	if limited[0].SessionID != "sess-d" {
		t.Errorf("limited[0].SessionID = %q, want sess-d (newest)", limited[0].SessionID)
	}

	since := base.Add(90 * time.Second)
	filtered, err := r.ListRecent(50, since)
	if err != nil {
		t.Fatalf("ListRecent since: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2 (sess-c, sess-d)", len(filtered))
	}
}

func TestRefineryRun_Insert_StampsFoldedAtWhenZero(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	r := NewRefineryRunRepo(pool, clk)

	run := &model.RefineryRun{SessionID: "sess-nofoldedat", ProjectID: "proj", Provider: "anthropic", Model: "haiku-4-5", Status: "ok"}
	if err := r.Insert(run); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !run.FoldedAt.Equal(clk.Now().UTC()) {
		t.Errorf("run.FoldedAt = %v, want stamped to clock time %v", run.FoldedAt, clk.Now().UTC())
	}

	got, err := r.ListRecent(1, time.Time{})
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 1 || !got[0].FoldedAt.Equal(clk.Now().UTC()) {
		t.Fatalf("got = %+v, want folded_at stamped to clock time", got)
	}
}
