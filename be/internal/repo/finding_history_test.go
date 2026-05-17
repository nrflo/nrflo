package repo

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFindingRepo_ListHistory_DescOrder verifies history is returned newest-first.
func TestFindingRepo_ListHistory_DescOrder(t *testing.T) {
	t.Parallel()
	r, clk := newFindingRepoTest(t)
	actor := Actor{Source: "agent"}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		clk.Set(base.Add(time.Duration(i) * time.Second))
		if err := r.Upsert("session", "hist-1", "k", json.RawMessage(`1`), Denorm{}, actor); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	hist, err := r.ListHistory("session", "hist-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history rows = %d, want 3", len(hist))
	}
	if !hist[0].CreatedAt.After(hist[1].CreatedAt) {
		t.Errorf("hist[0].CreatedAt = %v, want > hist[1] = %v", hist[0].CreatedAt, hist[1].CreatedAt)
	}
	if !hist[1].CreatedAt.After(hist[2].CreatedAt) {
		t.Errorf("hist[1].CreatedAt = %v, want > hist[2] = %v", hist[1].CreatedAt, hist[2].CreatedAt)
	}
}

// TestFindingRepo_ListHistory_Pagination verifies limit and offset work correctly.
func TestFindingRepo_ListHistory_Pagination(t *testing.T) {
	t.Parallel()
	r, clk := newFindingRepoTest(t)
	actor := Actor{Source: "agent"}
	base := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		clk.Set(base.Add(time.Duration(i) * time.Second))
		if err := r.Upsert("session", "pg-1", "k", json.RawMessage(`1`), Denorm{}, actor); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	page1, err := r.ListHistory("session", "pg-1", "", 2, 0)
	if err != nil {
		t.Fatalf("ListHistory page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page1 rows = %d, want 2", len(page1))
	}

	page2, err := r.ListHistory("session", "pg-1", "", 2, 2)
	if err != nil {
		t.Fatalf("ListHistory page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 rows = %d, want 2", len(page2))
	}

	page3, err := r.ListHistory("session", "pg-1", "", 2, 4)
	if err != nil {
		t.Fatalf("ListHistory page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3 rows = %d, want 1 (last partial page)", len(page3))
	}

	if page1[0].ID == page2[0].ID {
		t.Error("page1[0] and page2[0] should be different rows")
	}
}

// TestFindingRepo_ListHistory_KeyFilter verifies filter by key returns only that key's history.
func TestFindingRepo_ListHistory_KeyFilter(t *testing.T) {
	t.Parallel()
	r, _ := newFindingRepoTest(t)
	actor := Actor{Source: "agent"}

	r.Upsert("project", "kf-1", "k1", json.RawMessage(`"v1"`), Denorm{}, actor)  //nolint:errcheck
	r.Upsert("project", "kf-1", "k2", json.RawMessage(`"v2"`), Denorm{}, actor)  //nolint:errcheck
	r.Upsert("project", "kf-1", "k1", json.RawMessage(`"v1b"`), Denorm{}, actor) //nolint:errcheck

	all, err := r.ListHistory("project", "kf-1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all history rows = %d, want 3", len(all))
	}

	k1hist, err := r.ListHistory("project", "kf-1", "k1", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory k1: %v", err)
	}
	if len(k1hist) != 2 {
		t.Errorf("k1 history rows = %d, want 2", len(k1hist))
	}
	for _, h := range k1hist {
		if h.Key != "k1" {
			t.Errorf("expected key=k1, got %q", h.Key)
		}
	}

	k2hist, err := r.ListHistory("project", "kf-1", "k2", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory k2: %v", err)
	}
	if len(k2hist) != 1 {
		t.Errorf("k2 history rows = %d, want 1", len(k2hist))
	}
}

// TestFindingRepo_ListHistory_AppendOperation verifies append operations appear in history.
func TestFindingRepo_ListHistory_AppendOperation(t *testing.T) {
	t.Parallel()
	r, clk := newFindingRepoTest(t)
	actor := Actor{Source: "agent"}

	r.Upsert("project", "app-hist", "k", json.RawMessage(`"v1"`), Denorm{}, actor) //nolint:errcheck
	clk.Advance(time.Second)
	r.Append("project", "app-hist", "k", json.RawMessage(`"v2"`), Denorm{}, actor) //nolint:errcheck

	hist, err := r.ListHistory("project", "app-hist", "k", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history rows = %d, want 2", len(hist))
	}

	// Most recent is append
	if hist[0].Operation != "append" {
		t.Errorf("hist[0].Operation = %q, want append", hist[0].Operation)
	}
	// Old value is the previous value
	if !hist[0].OldValue.Valid || hist[0].OldValue.String != `"v1"` {
		t.Errorf("append old_value = %q (valid=%v), want \"v1\"", hist[0].OldValue.String, hist[0].OldValue.Valid)
	}
	if hist[1].Operation != "add" {
		t.Errorf("hist[1].Operation = %q, want add", hist[1].Operation)
	}
}

// TestFindingRepo_ListHistory_Empty verifies no error on empty history.
func TestFindingRepo_ListHistory_Empty(t *testing.T) {
	t.Parallel()
	r, _ := newFindingRepoTest(t)

	hist, err := r.ListHistory("session", "nonexistent", "", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory empty: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("expected 0 rows, got %d", len(hist))
	}
}
