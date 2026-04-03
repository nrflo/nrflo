package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleGetProject_PushAfterMergeDefaultFalse verifies that a project with no
// push_after_merge config entry returns push_after_merge: false in GET.
func TestHandleGetProject_PushAfterMergeDefaultFalse(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "push-proj-default")

	rr := httptest.NewRecorder()
	s.handleGetProject(rr, buildGetProjectReq(t, "push-proj-default"))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)

	raw, ok := resp["push_after_merge"]
	if !ok {
		t.Fatal("push_after_merge field absent from GET response")
	}
	val, ok := raw.(bool)
	if !ok {
		t.Fatalf("push_after_merge type = %T, want bool", raw)
	}
	if val {
		t.Errorf("push_after_merge = true, want false for new project")
	}
}

// TestHandlePatchProject_PushAfterMergeTrue verifies that PATCH with push_after_merge:true
// stores the config and GET returns true.
func TestHandlePatchProject_PushAfterMergeTrue(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "push-proj-true")

	body, _ := json.Marshal(map[string]interface{}{"push_after_merge": true})

	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "push-proj-true", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// PATCH response must include push_after_merge: true
	resp := decodeProjectResp(t, rr)
	if val, ok := resp["push_after_merge"].(bool); !ok || !val {
		t.Errorf("PATCH response push_after_merge = %v, want true", resp["push_after_merge"])
	}

	// GET must also return true
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "push-proj-true"))
	getRespBody := decodeProjectResp(t, rrGet)
	if val, ok := getRespBody["push_after_merge"].(bool); !ok || !val {
		t.Errorf("GET push_after_merge = %v, want true", getRespBody["push_after_merge"])
	}
}

// TestHandlePatchProject_PushAfterMergeFalse verifies that PATCH with push_after_merge:false
// clears the config and GET returns false.
func TestHandlePatchProject_PushAfterMergeFalse(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "push-proj-clear")

	// Set to true first.
	body, _ := json.Marshal(map[string]interface{}{"push_after_merge": true})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "push-proj-clear", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// Now clear with false.
	clearBody, _ := json.Marshal(map[string]interface{}{"push_after_merge": false})
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "push-proj-clear", string(clearBody)))
	if rr2.Code != http.StatusOK {
		t.Fatalf("clear PATCH status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	// GET must return false
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "push-proj-clear"))
	resp := decodeProjectResp(t, rrGet)
	if val, ok := resp["push_after_merge"].(bool); !ok || val {
		t.Errorf("GET push_after_merge = %v, want false after clear", resp["push_after_merge"])
	}
}

// TestHandlePatchProject_PushAfterMergeOmittedPreservesValue verifies that a PATCH
// without push_after_merge does not modify an existing true value.
func TestHandlePatchProject_PushAfterMergeOmittedPreservesValue(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "push-proj-omit")

	// Set to true.
	body, _ := json.Marshal(map[string]interface{}{"push_after_merge": true})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "push-proj-omit", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH only name — no push_after_merge field.
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "push-proj-omit", `{"name":"updated"}`))
	if rr2.Code != http.StatusOK {
		t.Fatalf("name PATCH status = %d, want 200", rr2.Code)
	}

	// GET must still return true.
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "push-proj-omit"))
	resp := decodeProjectResp(t, rrGet)
	if val, ok := resp["push_after_merge"].(bool); !ok || !val {
		t.Errorf("GET push_after_merge = %v after unrelated PATCH, want true (preserved)", resp["push_after_merge"])
	}
}

// TestHandleListProjects_IncludesPushAfterMerge verifies that GET /projects returns
// push_after_merge for each project: false by default, true when configured.
func TestHandleListProjects_IncludesPushAfterMerge(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "push-list-proj-1")
	seedTestProject(t, s, "push-list-proj-2")

	// Enable push on proj-1 only.
	body, _ := json.Marshal(map[string]interface{}{"push_after_merge": true})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "push-list-proj-1", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200", rr.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listRR := httptest.NewRecorder()
	s.handleListProjects(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listRR.Code, listRR.Body.String())
	}

	var listResp struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	byID := make(map[string]map[string]interface{})
	for _, p := range listResp.Projects {
		if id, _ := p["id"].(string); id != "" {
			byID[id] = p
		}
	}

	p1, ok := byID["push-list-proj-1"]
	if !ok {
		t.Fatal("push-list-proj-1 missing from list")
	}
	if val, ok := p1["push_after_merge"].(bool); !ok || !val {
		t.Errorf("push-list-proj-1: push_after_merge = %v, want true", p1["push_after_merge"])
	}

	p2, ok := byID["push-list-proj-2"]
	if !ok {
		t.Fatal("push-list-proj-2 missing from list")
	}
	if val, ok := p2["push_after_merge"].(bool); ok && val {
		t.Errorf("push-list-proj-2: push_after_merge = true, want false (default)")
	}
	if _, ok := p2["push_after_merge"]; !ok {
		t.Error("push-list-proj-2: push_after_merge field absent, want false")
	}
}
