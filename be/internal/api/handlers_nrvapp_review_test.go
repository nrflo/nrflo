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

// nrvappTestEnv holds a server + recorder for nrvapp handler tests.
type nrvappTestEnv struct {
	s   *Server
	rec *wsRecorder
}

func newNrvappTestEnv(t *testing.T) *nrvappTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "nrvapp_test.db")
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
	return &nrvappTestEnv{
		s:   &Server{pool: pool, clock: clock.Real(), wsHub: hub},
		rec: rec,
	}
}

func seedNrvappProject(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	_, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at)
		 VALUES (?, 'Test', '/tmp', datetime('now'), datetime('now'))`, projectID)
	if err != nil {
		t.Fatalf("seedNrvappProject(%q): %v", projectID, err)
	}
}

func insertNrvappReview(t *testing.T, pool *db.Pool, projectID, toolName, input string, draft *string) string {
	t.Helper()
	item := &model.NrvappReviewItem{
		ProjectID: projectID,
		ToolName:  toolName,
		Input:     input,
		Draft:     draft,
	}
	rr := repo.NewNrvappReviewRepo(pool, clock.Real())
	if err := rr.Insert(item); err != nil {
		t.Fatalf("insertNrvappReview: %v", err)
	}
	return item.ID
}

func nrvappReq(t *testing.T, handler func(http.ResponseWriter, *http.Request),
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

func TestHandleListNrvappReviews_MissingProject(t *testing.T) {
	env := newNrvappTestEnv(t)
	rr := nrvappReq(t, env.s.handleListNrvappReviews, http.MethodGet,
		"/api/v1/nrvapp/review", nil, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleCreateNrvappReview_MissingProject(t *testing.T) {
	env := newNrvappTestEnv(t)
	rr := nrvappReq(t, env.s.handleCreateNrvappReview, http.MethodPost,
		"/api/v1/nrvapp/review", nil, `{"tool_name":"t","input":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleGetNrvappReview_MissingProject(t *testing.T) {
	env := newNrvappTestEnv(t)
	rr := nrvappReq(t, env.s.handleGetNrvappReview, http.MethodGet,
		"/api/v1/nrvapp/review/rev-1", map[string]string{"id": "rev-1"}, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// --- List with status filter ---

func TestHandleListNrvappReviews_StatusFilter(t *testing.T) {
	env := newNrvappTestEnv(t)
	const pid = "proj-list-filter"
	seedNrvappProject(t, env.s.pool, pid)

	insertNrvappReview(t, env.s.pool, pid, "tool-a", `{"x":1}`, nil)
	id2 := insertNrvappReview(t, env.s.pool, pid, "tool-b", `{"x":2}`, nil)

	rr2 := repo.NewNrvappReviewRepo(env.s.pool, clock.Real())
	if err := rr2.Approve(id2, pid); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/nrvapp/review?status=pending", pid), nil)
	rec := httptest.NewRecorder()
	env.s.handleListNrvappReviews(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var items []*model.NrvappReviewItem
	json.NewDecoder(rec.Body).Decode(&items)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != model.ReviewStatusPending {
		t.Errorf("item status = %q, want %q", items[0].Status, model.ReviewStatusPending)
	}
}

// --- GET 404 and cross-project ---

func TestHandleGetNrvappReview_NotFound(t *testing.T) {
	env := newNrvappTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/nrvapp/review/nonexistent", "proj-x"), nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	env.s.handleGetNrvappReview(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetNrvappReview_CrossProject(t *testing.T) {
	env := newNrvappTestEnv(t)
	const ownerPID = "proj-owner-cross"
	const otherPID = "proj-other-cross"
	seedNrvappProject(t, env.s.pool, ownerPID)
	seedNrvappProject(t, env.s.pool, otherPID)
	itemID := insertNrvappReview(t, env.s.pool, ownerPID, "tool-a", `{"x":1}`, nil)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/nrvapp/review/"+itemID, otherPID), nil)
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleGetNrvappReview(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-project GET status = %d, want 404", rr.Code)
	}
}

// --- GET returns diff field when draft is set ---

func TestHandleGetNrvappReview_WithDiff(t *testing.T) {
	env := newNrvappTestEnv(t)
	const pid = "proj-diff"
	seedNrvappProject(t, env.s.pool, pid)
	draftVal := `{"a":2,"c":3}`
	itemID := insertNrvappReview(t, env.s.pool, pid, "tool-a", `{"a":1,"b":2}`, &draftVal)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/nrvapp/review/"+itemID, pid), nil)
	req.SetPathValue("id", itemID)
	rr := httptest.NewRecorder()
	env.s.handleGetNrvappReview(rr, req)

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

func TestHandleCreateNrvappReview_MissingToolName(t *testing.T) {
	env := newNrvappTestEnv(t)
	const pid = "proj-create-val"
	seedNrvappProject(t, env.s.pool, pid)
	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/nrvapp/review", pid),
		strings.NewReader(`{"input":"test"}`))
	rr := httptest.NewRecorder()
	env.s.handleCreateNrvappReview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "tool_name")
}

func TestHandleCreateNrvappReview_Success(t *testing.T) {
	env := newNrvappTestEnv(t)
	const pid = "proj-create-ok"
	seedNrvappProject(t, env.s.pool, pid)
	req := httptest.NewRequest(http.MethodPost, withProject("/api/v1/nrvapp/review", pid),
		strings.NewReader(`{"tool_name":"tool-a","input":"{\"x\":1}"}`))
	rr := httptest.NewRecorder()
	env.s.handleCreateNrvappReview(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var item model.NrvappReviewItem
	json.NewDecoder(rr.Body).Decode(&item)
	if item.ID == "" {
		t.Error("created item ID is empty")
	}
	if item.Status != model.ReviewStatusPending {
		t.Errorf("status = %q, want pending", item.Status)
	}
}
