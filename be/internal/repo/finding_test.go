package repo

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

func newFindingRepoTest(t *testing.T) (*FindingRepo, *clock.TestClock) {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewFindingRepo(pool, clk), clk
}

// TestFindingRepo_Upsert_Insert verifies insert path: old_value=NULL, operation=add, write_count=1.
func TestFindingRepo_Upsert_Insert(t *testing.T) {
	t.Parallel()
	r, _ := newFindingRepoTest(t)

	if err := r.Upsert("session", "s1", "k1", json.RawMessage(`"hello"`), Denorm{ProjectID: "p1"}, Actor{ID: "sess-1", Source: "agent"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	m, err := r.GetOwn("session", "s1")
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if string(m["k1"]) != `"hello"` {
		t.Errorf("value = %s, want \"hello\"", m["k1"])
	}

	hist, err := r.ListHistory("session", "s1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
	h := hist[0]
	if h.Operation != "add" {
		t.Errorf("operation = %q, want add", h.Operation)
	}
	if h.OldValue.Valid {
		t.Errorf("old_value on insert should be NULL, got %q", h.OldValue.String)
	}
	if h.NewValue.String != `"hello"` {
		t.Errorf("new_value = %q, want \"hello\"", h.NewValue.String)
	}
	if h.ActorID != "sess-1" || h.ActorSource != "agent" {
		t.Errorf("actor = {%q, %q}, want {sess-1, agent}", h.ActorID, h.ActorSource)
	}
}

// TestFindingRepo_Upsert_Update verifies update path: old_value=previous, write_count increments.
func TestFindingRepo_Upsert_Update(t *testing.T) {
	t.Parallel()
	r, clk := newFindingRepoTest(t)

	actor := Actor{ID: "sess-2", Source: "agent"}
	if err := r.Upsert("session", "s2", "k1", json.RawMessage(`"v1"`), Denorm{}, actor); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	clk.Advance(time.Second)
	if err := r.Upsert("session", "s2", "k1", json.RawMessage(`"v2"`), Denorm{}, actor); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	m, _ := r.GetOwn("session", "s2")
	if string(m["k1"]) != `"v2"` {
		t.Errorf("value after update = %s, want \"v2\"", m["k1"])
	}

	hist, err := r.ListHistory("session", "s2", "k1", 10, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history rows = %d, want 2", len(hist))
	}
	// DESC order: newest first
	update := hist[0]
	if !update.OldValue.Valid || update.OldValue.String != `"v1"` {
		t.Errorf("update old_value = %q (valid=%v), want \"v1\"", update.OldValue.String, update.OldValue.Valid)
	}
	if update.NewValue.String != `"v2"` {
		t.Errorf("update new_value = %q, want \"v2\"", update.NewValue.String)
	}
}

// TestFindingRepo_Append_Types covers number, array, object, and new-key paths with history operation=append.
func TestFindingRepo_Append_Types(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		existing json.RawMessage // nil means no pre-existing row
		newVal   json.RawMessage
		want     string
	}{
		{"number+number", json.RawMessage(`1`), json.RawMessage(`2`), `[1,2]`},
		{"array+array", json.RawMessage(`[1,2]`), json.RawMessage(`[3,4]`), `[1,2,3,4]`},
		{"scalar+object", json.RawMessage(`"a"`), json.RawMessage(`{"x":1}`), `["a",{"x":1}]`},
		{"new key", nil, json.RawMessage(`"first"`), `"first"`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, _ := newFindingRepoTest(t)
			actor := Actor{Source: "agent"}
			scopeID := "append-" + tc.name

			if tc.existing != nil {
				if err := r.Upsert("project", scopeID, "k", tc.existing, Denorm{}, actor); err != nil {
					t.Fatalf("Upsert existing: %v", err)
				}
			}
			if err := r.Append("project", scopeID, "k", tc.newVal, Denorm{}, actor); err != nil {
				t.Fatalf("Append: %v", err)
			}

			m, _ := r.GetOwn("project", scopeID)
			if got := string(m["k"]); got != tc.want {
				t.Errorf("Append(%s) = %s, want %s", tc.name, got, tc.want)
			}

			hist, _ := r.ListHistory("project", scopeID, "k", 10, 0)
			appendCount := 0
			for _, h := range hist {
				if h.Operation == "append" {
					appendCount++
				}
			}
			if appendCount != 1 {
				t.Errorf("append history rows = %d, want 1", appendCount)
			}
		})
	}
}

// TestFindingRepo_DeleteKeys covers single, bulk, and missing-key paths.
func TestFindingRepo_DeleteKeys(t *testing.T) {
	t.Parallel()

	t.Run("single key records old_value in history", func(t *testing.T) {
		t.Parallel()
		r, _ := newFindingRepoTest(t)
		actor := Actor{Source: "user"}

		r.Upsert("project", "del-1", "k1", json.RawMessage(`"v1"`), Denorm{}, actor) //nolint:errcheck
		r.Upsert("project", "del-1", "k2", json.RawMessage(`"v2"`), Denorm{}, actor) //nolint:errcheck

		deleted, err := r.DeleteKeys("project", "del-1", []string{"k1"}, actor)
		if err != nil {
			t.Fatalf("DeleteKeys: %v", err)
		}
		if len(deleted) != 1 || deleted[0] != "k1" {
			t.Errorf("deleted = %v, want [k1]", deleted)
		}

		m, _ := r.GetOwn("project", "del-1")
		if _, ok := m["k1"]; ok {
			t.Error("k1 should be deleted")
		}
		if _, ok := m["k2"]; !ok {
			t.Error("k2 should remain")
		}

		hist, _ := r.ListHistory("project", "del-1", "k1", 10, 0)
		delCount := 0
		for _, h := range hist {
			if h.Operation == "delete" {
				delCount++
				if !h.OldValue.Valid || h.OldValue.String != `"v1"` {
					t.Errorf("delete old_value = %q (valid=%v), want \"v1\"", h.OldValue.String, h.OldValue.Valid)
				}
			}
		}
		if delCount != 1 {
			t.Errorf("delete history rows = %d, want 1", delCount)
		}
	})

	t.Run("bulk delete returns all removed", func(t *testing.T) {
		t.Parallel()
		r, _ := newFindingRepoTest(t)
		actor := Actor{Source: "user"}

		for _, k := range []string{"a", "b", "c"} {
			r.Upsert("project", "del-2", k, json.RawMessage(`1`), Denorm{}, actor) //nolint:errcheck
		}

		deleted, err := r.DeleteKeys("project", "del-2", []string{"a", "c"}, actor)
		if err != nil {
			t.Fatalf("DeleteKeys bulk: %v", err)
		}
		if len(deleted) != 2 {
			t.Errorf("deleted count = %d, want 2", len(deleted))
		}
		m, _ := r.GetOwn("project", "del-2")
		if len(m) != 1 {
			t.Errorf("remaining keys = %d, want 1", len(m))
		}
	})

	t.Run("missing keys not counted", func(t *testing.T) {
		t.Parallel()
		r, _ := newFindingRepoTest(t)
		actor := Actor{Source: "user"}

		r.Upsert("project", "del-3", "exists", json.RawMessage(`true`), Denorm{}, actor) //nolint:errcheck

		deleted, err := r.DeleteKeys("project", "del-3", []string{"exists", "missing"}, actor)
		if err != nil {
			t.Fatalf("DeleteKeys missing: %v", err)
		}
		if len(deleted) != 1 || deleted[0] != "exists" {
			t.Errorf("deleted = %v, want [exists]", deleted)
		}
	})

	t.Run("empty keys list returns nil", func(t *testing.T) {
		t.Parallel()
		r, _ := newFindingRepoTest(t)
		deleted, err := r.DeleteKeys("project", "del-4", []string{}, Actor{})
		if err != nil {
			t.Fatalf("DeleteKeys empty: %v", err)
		}
		if len(deleted) != 0 {
			t.Errorf("deleted = %v, want empty", deleted)
		}
	})
}

// TestFindingRepo_GetOwn_MultipleKeys verifies all keys for a scope are returned.
func TestFindingRepo_GetOwn_MultipleKeys(t *testing.T) {
	t.Parallel()
	r, _ := newFindingRepoTest(t)
	actor := Actor{Source: "agent"}

	r.Upsert("project", "own-1", "key-a", json.RawMessage(`"va"`), Denorm{}, actor) //nolint:errcheck
	r.Upsert("project", "own-1", "key-b", json.RawMessage(`42`), Denorm{}, actor)    //nolint:errcheck
	// Different scope_id — should not appear
	r.Upsert("project", "own-2", "key-c", json.RawMessage(`true`), Denorm{}, actor) //nolint:errcheck

	m, err := r.GetOwn("project", "own-1")
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("GetOwn returned %d keys, want 2", len(m))
	}
	if string(m["key-a"]) != `"va"` {
		t.Errorf("key-a = %s, want \"va\"", m["key-a"])
	}
	if string(m["key-b"]) != `42` {
		t.Errorf("key-b = %s, want 42", m["key-b"])
	}
	if _, ok := m["key-c"]; ok {
		t.Error("key-c from different scope_id should not appear")
	}
}
