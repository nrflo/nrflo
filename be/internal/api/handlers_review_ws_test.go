package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/ws"
)

// TestHandlePatchReview_UpdatesDraftAndEmitsEvent verifies that PATCH
// updates the draft field and broadcasts EventReviewUpdated.
func TestHandlePatchReview_UpdatesDraftAndEmitsEvent(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-patch"
	seedReviewProject(t, env.s.pool, pid)
	itemID := insertReview(t, env.s.pool, pid, "tool-a", `{"a":1}`, nil)

	req := httptest.NewRequest(http.MethodPatch, withProject("/api/v1/review/"+itemID, pid),
		strings.NewReader(`{"draft":"{\"a\":2}"}`))
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handlePatchReview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var item model.ReviewItem
	json.NewDecoder(rr.Body).Decode(&item)
	if item.Draft == nil || *item.Draft != `{"a":2}` {
		t.Errorf("Draft = %v, want {\"a\":2}", item.Draft)
	}

	ev := env.rec.waitEvent(t, ws.EventReviewUpdated)
	if ev.Data["review_item_id"] != itemID {
		t.Errorf("event review_item_id = %v, want %q", ev.Data["review_item_id"], itemID)
	}
	if ev.Data["status"] != "pending" {
		t.Errorf("event status = %v, want pending", ev.Data["status"])
	}
}

// TestHandlePatchReview_NotFound returns 404 for unknown item.
func TestHandlePatchReview_NotFound(t *testing.T) {
	env := newReviewTestEnv(t)
	req := httptest.NewRequest(http.MethodPatch, withProject("/api/v1/review/ghost", "proj-any"),
		strings.NewReader(`{"draft":"x"}`))
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	env.s.handlePatchReview(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("PATCH unknown status = %d, want 404", rr.Code)
	}
}

// TestHandleApproveReview_CopiesDraftToOutput verifies that approve sets
// output = draft when output was previously nil, and broadcasts the event.
func TestHandleApproveReview_CopiesDraftToOutput(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-approve"
	seedReviewProject(t, env.s.pool, pid)
	draft := `{"result":"ok"}`
	itemID := insertReview(t, env.s.pool, pid, "tool-a", `{"a":1}`, &draft)

	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/review/"+itemID+"/approve", pid), nil)
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleApproveReview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var item model.ReviewItem
	json.NewDecoder(rr.Body).Decode(&item)
	if item.Status != model.ReviewStatusApproved {
		t.Errorf("status = %q, want approved", item.Status)
	}
	if item.Output == nil || *item.Output != draft {
		t.Errorf("output = %v, want %q (copied from draft)", item.Output, draft)
	}

	ev := env.rec.waitEvent(t, ws.EventReviewUpdated)
	if ev.Data["status"] != "approved" {
		t.Errorf("event status = %v, want approved", ev.Data["status"])
	}
}

// TestHandleApproveReview_MissingProject returns 400.
func TestHandleApproveReview_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/review/rev-1/approve", nil)
	req.SetPathValue("id", "rev-1")
	rr := httptest.NewRecorder()
	env.s.handleApproveReview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// TestHandleRejectReview_SetsReasonAndEmitsEvent verifies reject sets
// reject_reason and broadcasts the event.
func TestHandleRejectReview_SetsReasonAndEmitsEvent(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-reject"
	seedReviewProject(t, env.s.pool, pid)
	itemID := insertReview(t, env.s.pool, pid, "tool-a", `{"a":1}`, nil)

	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/review/"+itemID+"/reject", pid),
		strings.NewReader(`{"reason":"output was wrong"}`))
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleRejectReview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("reject status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var item model.ReviewItem
	json.NewDecoder(rr.Body).Decode(&item)
	if item.Status != model.ReviewStatusRejected {
		t.Errorf("status = %q, want rejected", item.Status)
	}
	if item.RejectReason == nil || *item.RejectReason != "output was wrong" {
		t.Errorf("reject_reason = %v, want 'output was wrong'", item.RejectReason)
	}

	ev := env.rec.waitEvent(t, ws.EventReviewUpdated)
	if ev.Data["status"] != "rejected" {
		t.Errorf("event status = %v, want rejected", ev.Data["status"])
	}
}

// TestHandleRejectReview_MissingProject returns 400.
func TestHandleRejectReview_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/review/rev-1/reject",
		strings.NewReader(`{"reason":"x"}`))
	req.SetPathValue("id", "rev-1")
	rr := httptest.NewRecorder()
	env.s.handleRejectReview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}
