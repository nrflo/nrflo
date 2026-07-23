package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// seedStepRotation inserts an agent_step_cursors row whose completed JSON
// carries one Rotated=true entry, for the step_rotation Activity-view merge.
func seedStepRotation(t *testing.T, s *Server, wfiID, nodeID, stepID, sessionID, completedAt string) {
	t.Helper()
	completed := `[{"step_id":"` + stepID + `","session_id":"` + sessionID + `","completed_at":"` + completedAt + `","rotated":true}]`
	if _, err := s.pool.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, ?, '[]', 1, 0, ?, '{}', ?, ?)`,
		wfiID, nodeID, completed, completedAt, completedAt,
	); err != nil {
		t.Fatalf("seed step rotation: %v", err)
	}
}

// TestHandleListSystemAgentRuns_StepRotationInterleavesWithOtherSources
// verifies a step_rotation item merges by created_at alongside agent_session
// and refinery_fold items, newest-first, discriminated by kind.
func TestHandleListSystemAgentRuns_StepRotationInterleavesWithOtherSources(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRunsAgentSession(t, s, "sess-1", wfiID, base.Format(time.RFC3339Nano), 1)
	seedStepRotation(t, s, wfiID, "node-a", "s1", "sess-2", base.Add(time.Minute).Format(time.RFC3339Nano))
	seedRefineryRun(t, s, "sess-3", base.Add(2*time.Minute).Format(time.RFC3339Nano))
	seedRunsAgentSession(t, s, "sess-4", wfiID, base.Add(3*time.Minute).Format(time.RFC3339Nano), 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeRunsResponse(t, rr)
	if len(items) != 4 {
		t.Fatalf("len(items) = %d, want 4", len(items))
	}
	wantOrder := []string{"agent_session", "refinery_fold", "step_rotation", "agent_session"}
	for i, want := range wantOrder {
		if items[i]["kind"] != want {
			t.Errorf("items[%d].kind = %v, want %q (newest-first interleave)", i, items[i]["kind"], want)
		}
	}
	rotationItem := items[2]
	if rotationItem["step_id"] != "s1" {
		t.Errorf("step_rotation item step_id = %v, want s1", rotationItem["step_id"])
	}
	if rotationItem["session_id"] != "sess-2" {
		t.Errorf("step_rotation item session_id = %v, want sess-2", rotationItem["session_id"])
	}
	if rotationItem["status"] != "rotated" {
		t.Errorf("step_rotation item status = %v, want rotated", rotationItem["status"])
	}
}

// TestHandleListSystemAgentRuns_StepRotationRespectsLimitTrim verifies a
// step_rotation entry is subject to the same overall limit trim as the other
// two sources — not exempt as a third independently-capped source.
func TestHandleListSystemAgentRuns_StepRotationRespectsLimitTrim(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedStepRotation(t, s, wfiID, "node-a", "old", "sess-old", base.Format(time.RFC3339Nano))
	seedRunsAgentSession(t, s, "sess-mid", wfiID, base.Add(time.Minute).Format(time.RFC3339Nano), 1)
	seedStepRotation(t, s, wfiID, "node-b", "new", "sess-new", base.Add(2*time.Minute).Format(time.RFC3339Nano))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs?limit=2", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	items, limit := decodeRunsResponse(t, rr)
	if limit != 2 {
		t.Errorf("limit = %d, want 2", limit)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (trimmed across merged sources)", len(items))
	}
	if items[0]["step_id"] != "new" || items[1]["session_id"] != "sess-mid" {
		t.Errorf("items = %v, want [new rotation, sess-mid] (2 newest, oldest rotation dropped)", items)
	}
}

// TestHandleListSystemAgentRuns_StepRotationOnlyRotatedEntriesSurfaced
// verifies a non-rotated CompletedStep entry never surfaces as a
// step_rotation item.
func TestHandleListSystemAgentRuns_StepRotationOnlyRotatedEntriesSurfaced(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	wfiID := seedRunsProjectAndWFI(t, s)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	completed := `[{"step_id":"s1","completed_at":"` + now + `","rotated":false}]`
	if _, err := s.pool.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, 'node-a', '[]', 1, 0, ?, '{}', ?, ?)`,
		wfiID, completed, now, now,
	); err != nil {
		t.Fatalf("seed non-rotated cursor: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	var body struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("items = %s, want empty (no rotated completed steps)", body.Items)
	}
}
