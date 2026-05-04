package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// wsRecorder captures WS events for handler tests.
type wsRecorder struct {
	mu     sync.Mutex
	events []*ws.Event
	ch     chan *ws.Event
}

func (r *wsRecorder) OnEvent(e *ws.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	select {
	case r.ch <- e:
	default:
	}
}

// waitEvent polls until an event of the given type is seen or the timeout elapses.
func (r *wsRecorder) waitEvent(t *testing.T, eventType string) *ws.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, ev := range r.events {
			if ev.Type == eventType {
				r.mu.Unlock()
				return ev
			}
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for WS event %q", eventType)
	return nil
}

// reviewTestEnv holds a server + recorder for review handler tests.
type reviewTestEnv struct {
	s   *Server
	rec *wsRecorder
}

func newReviewTestEnv(t *testing.T) *reviewTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "review_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	rec := &wsRecorder{ch: make(chan *ws.Event, 32)}
	hub.RegisterListener(rec)
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})
	return &reviewTestEnv{
		s:   &Server{pool: pool, clock: clock.Real(), wsHub: hub},
		rec: rec,
	}
}

func seedReviewProject(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at)
		 VALUES (?, 'Test', '/tmp', datetime('now'), datetime('now'))`, projectID)
	if err != nil {
		t.Fatalf("seedReviewProject(%q): %v", projectID, err)
	}
}

func insertReview(t *testing.T, pool *db.Pool, projectID, toolName, input string, draft *string) string {
	t.Helper()
	item := &model.ReviewItem{
		ProjectID: projectID,
		ToolName:  toolName,
		Input:     input,
		Draft:     draft,
	}
	rr := repo.NewReviewRepo(pool, clock.Real())
	if err := rr.Insert(item); err != nil {
		t.Fatalf("insertReview: %v", err)
	}
	return item.ID
}

func reviewReq(t *testing.T, handler func(http.ResponseWriter, *http.Request),
	method, url string, pv map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	for k, v := range pv {
		req.SetPathValue(k, v)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

// --- Missing X-Project header ---

func TestHandleListReviews_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	rr := reviewReq(t, env.s.handleListReviews, http.MethodGet,
		"/api/v1/review", nil, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleCreateReview_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	rr := reviewReq(t, env.s.handleCreateReview, http.MethodPost,
		"/api/v1/review", nil, `{"tool_name":"t","input":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleGetReview_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	rr := reviewReq(t, env.s.handleGetReview, http.MethodGet,
		"/api/v1/review/rev-1", map[string]string{"id": "rev-1"}, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// --- List with status filter ---

func TestHandleListReviews_StatusFilter(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-list-filter"
	seedReviewProject(t, env.s.pool, pid)

	insertReview(t, env.s.pool, pid, "tool-a", `{"x":1}`, nil)
	id2 := insertReview(t, env.s.pool, pid, "tool-b", `{"x":2}`, nil)

	rr2 := repo.NewReviewRepo(env.s.pool, clock.Real())
	if err := rr2.Approve(id2, pid); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/review?status=pending", pid), nil)
	rec := httptest.NewRecorder()
	env.s.handleListReviews(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var items []*model.ReviewItem
	json.NewDecoder(rec.Body).Decode(&items)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != model.ReviewStatusPending {
		t.Errorf("item status = %q, want %q", items[0].Status, model.ReviewStatusPending)
	}
}

// --- GET 404 and cross-project ---

func TestHandleGetReview_NotFound(t *testing.T) {
	env := newReviewTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/review/nonexistent", "proj-x"), nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	env.s.handleGetReview(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetReview_CrossProject(t *testing.T) {
	env := newReviewTestEnv(t)
	const ownerPID = "proj-owner-cross"
	const otherPID = "proj-other-cross"
	seedReviewProject(t, env.s.pool, ownerPID)
	seedReviewProject(t, env.s.pool, otherPID)
	itemID := insertReview(t, env.s.pool, ownerPID, "tool-a", `{"x":1}`, nil)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/review/"+itemID, otherPID), nil)
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleGetReview(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-project GET status = %d, want 404", rr.Code)
	}
}

// --- GET returns diff field when draft is set ---

func TestHandleGetReview_WithDiff(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-diff"
	seedReviewProject(t, env.s.pool, pid)
	draftVal := `{"a":2,"c":3}`
	itemID := insertReview(t, env.s.pool, pid, "tool-a", `{"a":1,"b":2}`, &draftVal)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/review/"+itemID, pid), nil)
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleGetReview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	diff, ok := resp["diff"]
	if !ok || diff == nil {
		t.Errorf("response missing or nil diff field; got %v", resp)
	}
}

// --- Create validation ---

func TestHandleCreateReview_MissingToolName(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-create-val"
	seedReviewProject(t, env.s.pool, pid)
	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/review", pid),
		strings.NewReader(`{"input":"test"}`))
	rr := httptest.NewRecorder()
	env.s.handleCreateReview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "tool_name")
}

func TestHandleCreateReview_Success(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-create-ok"
	seedReviewProject(t, env.s.pool, pid)
	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/review", pid),
		strings.NewReader(`{"tool_name":"tool-a","input":"{\"x\":1}"}`))
	rr := httptest.NewRecorder()
	env.s.handleCreateReview(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var item model.ReviewItem
	json.NewDecoder(rr.Body).Decode(&item)
	if item.ID == "" {
		t.Error("created item ID is empty")
	}
	if item.Status != model.ReviewStatusPending {
		t.Errorf("status = %q, want pending", item.Status)
	}
}
