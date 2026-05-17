package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/repo"
)

// TestHandleListFindingsHistory_OrderDescByCreatedAt verifies history is returned newest-first.
func TestHandleListFindingsHistory_OrderDescByCreatedAt(t *testing.T) {
	s, clk := newHistServerWithClock(t)
	seedProjectForFindings(t, s, "proj-order")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := repo.NewFindingRepo(s.pool, s.clock)
	actor := repo.Actor{Source: "system"}
	for i := 0; i < 3; i++ {
		clk.Set(base.Add(time.Duration(i) * time.Second))
		if err := r.Upsert("project", "proj-order", "k", []byte(`1`), repo.Denorm{}, actor); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-order"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	resp := decodeHistResp(t, rr)
	if len(resp.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(resp.Items))
	}

	for i := 1; i < len(resp.Items); i++ {
		t0, _ := time.Parse(time.RFC3339, resp.Items[i-1]["created_at"].(string))
		t1, _ := time.Parse(time.RFC3339, resp.Items[i]["created_at"].(string))
		if !t0.After(t1) {
			t.Errorf("items[%d].created_at %v not after items[%d].created_at %v", i-1, t0, i, t1)
		}
	}
}

// TestHandleListFindingsHistory_KeyFilter verifies ?key=foo narrows results.
func TestHandleListFindingsHistory_KeyFilter(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-kf")
	upsertHistFinding(t, s, "project", "proj-kf", "alpha")
	upsertHistFinding(t, s, "project", "proj-kf", "beta")
	upsertHistFinding(t, s, "project", "proj-kf", "alpha") // second write → second history row

	// Without filter: all 3 history rows.
	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-kf"))
	resp := decodeHistResp(t, rr)
	if len(resp.Items) != 3 {
		t.Errorf("all items = %d, want 3", len(resp.Items))
	}

	// With key=alpha: only 2 rows.
	rr2 := httptest.NewRecorder()
	s.handleListFindingsHistory(rr2, histReq(t, "scope=project&scope_id=proj-kf&key=alpha"))
	resp2 := decodeHistResp(t, rr2)
	if len(resp2.Items) != 2 {
		t.Errorf("alpha items = %d, want 2", len(resp2.Items))
	}
	for _, item := range resp2.Items {
		if item["key"] != "alpha" {
			t.Errorf("item key = %v, want alpha", item["key"])
		}
	}
}

// TestHandleListFindingsHistory_Pagination verifies limit+offset iterates all rows.
func TestHandleListFindingsHistory_Pagination(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-pg")

	for i := 0; i < 6; i++ {
		upsertHistFinding(t, s, "project", "proj-pg", fmt.Sprintf("k%d", i))
	}

	rr1 := httptest.NewRecorder()
	s.handleListFindingsHistory(rr1, histReq(t, "scope=project&scope_id=proj-pg&limit=5&offset=0"))
	r1 := decodeHistResp(t, rr1)
	if len(r1.Items) != 5 {
		t.Errorf("page1 items = %d, want 5", len(r1.Items))
	}
	if r1.Limit != 5 {
		t.Errorf("page1 limit = %v, want 5", r1.Limit)
	}
	if r1.Offset != 0 {
		t.Errorf("page1 offset = %v, want 0", r1.Offset)
	}

	rr2 := httptest.NewRecorder()
	s.handleListFindingsHistory(rr2, histReq(t, "scope=project&scope_id=proj-pg&limit=5&offset=5"))
	r2 := decodeHistResp(t, rr2)
	if len(r2.Items) != 1 {
		t.Errorf("page2 items = %d, want 1", len(r2.Items))
	}
	if r2.Limit != 5 {
		t.Errorf("page2 limit = %v, want 5", r2.Limit)
	}
	if r2.Offset != 5 {
		t.Errorf("page2 offset = %v, want 5", r2.Offset)
	}

	// Pages must not overlap.
	if len(r1.Items) > 0 && len(r2.Items) > 0 {
		if r1.Items[0]["id"] == r2.Items[0]["id"] {
			t.Error("page1 and page2 overlap on id")
		}
	}
}

// TestHandleListFindingsHistory_LimitCap verifies limit > 200 is clamped to 200.
func TestHandleListFindingsHistory_LimitCap(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-cap")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-cap&limit=999"))
	resp := decodeHistResp(t, rr)
	if resp.Limit != 200 {
		t.Errorf("limit = %v, want 200 (capped)", resp.Limit)
	}
}

// TestHandleListFindingsHistory_DefaultLimit verifies limit defaults to 50 when omitted.
func TestHandleListFindingsHistory_DefaultLimit(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-deflimit")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-deflimit"))
	resp := decodeHistResp(t, rr)
	if resp.Limit != 50 {
		t.Errorf("default limit = %v, want 50", resp.Limit)
	}
}

// TestHandleListFindingsHistory_WFIScope verifies scope=workflow_instance resolves project.
func TestHandleListFindingsHistory_WFIScope(t *testing.T) {
	s := newHistServer(t)
	seedWFIForHistory(t, s, "wfi-hist-1", "proj-wfi-scope")
	upsertHistFinding(t, s, "workflow_instance", "wfi-hist-1", "phase")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=workflow_instance&scope_id=wfi-hist-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	resp := decodeHistResp(t, rr)
	if len(resp.Items) != 1 {
		t.Errorf("items = %d, want 1", len(resp.Items))
	}
	if len(resp.Items) > 0 && resp.Items[0]["key"] != "phase" {
		t.Errorf("key = %v, want phase", resp.Items[0]["key"])
	}
}

// TestHandleListFindingsHistory_SessionScope verifies scope=session resolves project.
func TestHandleListFindingsHistory_SessionScope(t *testing.T) {
	s := newHistServer(t)
	seedSessionForHistory(t, s, "sess-hist-1", "proj-sess-scope")
	upsertHistFinding(t, s, "session", "sess-hist-1", "result")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=session&scope_id=sess-hist-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	resp := decodeHistResp(t, rr)
	if len(resp.Items) != 1 {
		t.Errorf("items = %d, want 1", len(resp.Items))
	}
}

// TestHandleListFindingsHistory_NullStringsFlattened verifies old_value/new_value are
// null or plain string, not the {String:...,Valid:...} struct form.
func TestHandleListFindingsHistory_NullStringsFlattened(t *testing.T) {
	s, clk := newHistServerWithClock(t)
	seedProjectForFindings(t, s, "proj-nullstr")
	r := repo.NewFindingRepo(s.pool, s.clock)
	actor := repo.Actor{Source: "system"}

	clk.Set(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := r.Upsert("project", "proj-nullstr", "k1", []byte(`"first"`), repo.Denorm{}, actor); err != nil {
		t.Fatalf("Upsert1: %v", err)
	}
	clk.Advance(time.Second)
	if err := r.Upsert("project", "proj-nullstr", "k1", []byte(`"second"`), repo.Denorm{}, actor); err != nil {
		t.Fatalf("Upsert2: %v", err)
	}

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-nullstr"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var raw struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Items) < 2 {
		t.Fatalf("items = %d, want 2", len(raw.Items))
	}

	// Most recent item: second upsert → old_value should be a plain JSON string.
	var latest map[string]json.RawMessage
	if err := json.Unmarshal(raw.Items[0], &latest); err != nil {
		t.Fatalf("unmarshal latest: %v", err)
	}
	oldVal, ok := latest["old_value"]
	if !ok || string(oldVal) == "null" {
		t.Fatalf("latest item should have non-null old_value, got: %s", oldVal)
	}
	var strVal string
	if err := json.Unmarshal(oldVal, &strVal); err != nil {
		t.Errorf("old_value should be a string, got %s: %v", oldVal, err)
	}

	// First item: initial insert → old_value should be null (not an object).
	var first map[string]json.RawMessage
	if err := json.Unmarshal(raw.Items[1], &first); err != nil {
		t.Fatalf("unmarshal first: %v", err)
	}
	if firstOld, ok := first["old_value"]; ok && string(firstOld) != "null" {
		t.Errorf("first item old_value should be null, got %s", firstOld)
	}
}
