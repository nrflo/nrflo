package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHandleListSystemAgentRuns_MergedNewestFirst verifies the merged
// response interleaves agent_session and refinery_fold rows newest-first,
// discriminated by kind, and excludes an ordinary non-system phase session.
func TestHandleListSystemAgentRuns_MergedNewestFirst(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRunsAgentSession(t, s, "sess-1", wfiID, base.Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-2", base.Add(time.Minute).Format(time.RFC3339Nano))
	seedRunsAgentSession(t, s, "sess-3", wfiID, base.Add(2*time.Minute).Format(time.RFC3339Nano), 2)
	seedOrdinaryPhaseSession(t, s, "sess-ordinary", wfiID, base.Add(3*time.Minute).Format(time.RFC3339Nano))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	items, limit := decodeRunsResponse(t, rr)
	if limit != 50 {
		t.Errorf("limit = %d, want default 50", limit)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3 (sess-1, sess-2 fold, sess-3; sess-ordinary excluded)", len(items))
	}
	// Newest first: sess-3 (session), sess-2 (fold), sess-1 (session).
	wantOrder := []string{"sess-3", "sess-2", "sess-1"}
	for i, want := range wantOrder {
		if items[i]["session_id"] != want {
			t.Errorf("items[%d].session_id = %v, want %q (newest-first)", i, items[i]["session_id"], want)
		}
	}
	if items[1]["kind"] != "refinery_fold" {
		t.Errorf("items[1].kind = %v, want refinery_fold", items[1]["kind"])
	}
	if items[0]["kind"] != "agent_session" {
		t.Errorf("items[0].kind = %v, want agent_session", items[0]["kind"])
	}
	for _, it := range items {
		if it["session_id"] == "sess-ordinary" {
			t.Error("ordinary non-system phase session present in response, want excluded")
		}
	}
}

// TestHandleListSystemAgentRuns_LimitTruncatesMerged verifies limit truncates
// the merged list as a whole, not each source independently.
func TestHandleListSystemAgentRuns_LimitTruncatesMerged(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRunsAgentSession(t, s, "sess-a", wfiID, base.Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-b", base.Add(time.Minute).Format(time.RFC3339Nano))
	seedRunsAgentSession(t, s, "sess-c", wfiID, base.Add(2*time.Minute).Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-d", base.Add(3*time.Minute).Format(time.RFC3339Nano))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs?limit=2", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	items, limit := decodeRunsResponse(t, rr)
	if limit != 2 {
		t.Errorf("limit = %d, want 2", limit)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (merged truncation)", len(items))
	}
	if items[0]["session_id"] != "sess-d" || items[1]["session_id"] != "sess-c" {
		t.Errorf("items = %v, want [sess-d, sess-c] (2 newest across merged sources)", items)
	}
}

// TestHandleListSystemAgentRuns_SinceFiltersBothSources verifies since
// filters both the agent_session and refinery_fold sources.
func TestHandleListSystemAgentRuns_SinceFiltersBothSources(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRunsAgentSession(t, s, "sess-old", wfiID, base.Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-old-fold", base.Format(time.RFC3339Nano))
	seedRunsAgentSession(t, s, "sess-new", wfiID, base.Add(time.Hour).Format(time.RFC3339Nano), 1)
	seedRefineryRun(t, s, "sess-new-fold", base.Add(time.Hour).Format(time.RFC3339Nano))

	since := base.Add(30 * time.Minute).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs?since="+since, nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	items, _ := decodeRunsResponse(t, rr)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (only the two 'new' rows survive since)", len(items))
	}
	for _, it := range items {
		sid := it["session_id"]
		if sid == "sess-old" || sid == "sess-old-fold" {
			t.Errorf("old row %v present, want filtered by since", sid)
		}
	}
}
