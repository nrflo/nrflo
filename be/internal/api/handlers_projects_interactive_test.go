package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleGetProject_InteractiveCLIModeDefaultFalse verifies that a project
// with no interactive_cli_mode config entry returns interactive_cli_mode: false in GET.
func TestHandleGetProject_InteractiveCLIModeDefaultFalse(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "icm-proj-default")

	rr := httptest.NewRecorder()
	s.handleGetProject(rr, buildGetProjectReq(t, "icm-proj-default"))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)

	raw, ok := resp["interactive_cli_mode"]
	if !ok {
		t.Fatal("interactive_cli_mode field absent from GET response")
	}
	val, ok := raw.(bool)
	if !ok {
		t.Fatalf("interactive_cli_mode type = %T, want bool", raw)
	}
	if val {
		t.Errorf("interactive_cli_mode = true, want false for new project")
	}
}

// TestHandlePatchProject_InteractiveCLIModeTrue verifies that PATCH with
// interactive_cli_mode:true stores the config and GET returns true.
func TestHandlePatchProject_InteractiveCLIModeTrue(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "icm-proj-true")

	body, _ := json.Marshal(map[string]interface{}{"interactive_cli_mode": true})

	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "icm-proj-true", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// PATCH response must include interactive_cli_mode: true
	resp := decodeProjectResp(t, rr)
	if val, ok := resp["interactive_cli_mode"].(bool); !ok || !val {
		t.Errorf("PATCH response interactive_cli_mode = %v, want true", resp["interactive_cli_mode"])
	}

	// GET must also return true
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "icm-proj-true"))
	getRespBody := decodeProjectResp(t, rrGet)
	if val, ok := getRespBody["interactive_cli_mode"].(bool); !ok || !val {
		t.Errorf("GET interactive_cli_mode = %v, want true", getRespBody["interactive_cli_mode"])
	}
}

// TestHandlePatchProject_InteractiveCLIModeFalse verifies that PATCH with
// interactive_cli_mode:false clears the config and GET returns false.
func TestHandlePatchProject_InteractiveCLIModeFalse(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "icm-proj-clear")

	// Set to true first.
	body, _ := json.Marshal(map[string]interface{}{"interactive_cli_mode": true})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "icm-proj-clear", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// Now clear with false.
	clearBody, _ := json.Marshal(map[string]interface{}{"interactive_cli_mode": false})
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "icm-proj-clear", string(clearBody)))
	if rr2.Code != http.StatusOK {
		t.Fatalf("clear PATCH status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	// GET must return false
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "icm-proj-clear"))
	resp := decodeProjectResp(t, rrGet)
	if val, ok := resp["interactive_cli_mode"].(bool); !ok || val {
		t.Errorf("GET interactive_cli_mode = %v, want false after clear", resp["interactive_cli_mode"])
	}
}

// TestHandlePatchProject_InteractiveCLIModeOmittedPreservesValue verifies that a
// PATCH without interactive_cli_mode does not modify an existing true value.
func TestHandlePatchProject_InteractiveCLIModeOmittedPreservesValue(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "icm-proj-omit")

	// Set to true.
	body, _ := json.Marshal(map[string]interface{}{"interactive_cli_mode": true})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "icm-proj-omit", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH only name — no interactive_cli_mode field.
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "icm-proj-omit", `{"name":"updated"}`))
	if rr2.Code != http.StatusOK {
		t.Fatalf("name PATCH status = %d, want 200", rr2.Code)
	}

	// GET must still return true.
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "icm-proj-omit"))
	resp := decodeProjectResp(t, rrGet)
	if val, ok := resp["interactive_cli_mode"].(bool); !ok || !val {
		t.Errorf("GET interactive_cli_mode = %v after unrelated PATCH, want true (preserved)", resp["interactive_cli_mode"])
	}
}

// TestHandleListProjects_IncludesInteractiveCLIMode verifies that GET /projects
// returns interactive_cli_mode for each project: false by default, true when configured.
func TestHandleListProjects_IncludesInteractiveCLIMode(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "icm-list-proj-1")
	seedTestProject(t, s, "icm-list-proj-2")

	// Enable interactive_cli_mode on proj-1 only.
	body, _ := json.Marshal(map[string]interface{}{"interactive_cli_mode": true})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "icm-list-proj-1", string(body)))
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

	p1, ok := byID["icm-list-proj-1"]
	if !ok {
		t.Fatal("icm-list-proj-1 missing from list")
	}
	if val, ok := p1["interactive_cli_mode"].(bool); !ok || !val {
		t.Errorf("icm-list-proj-1: interactive_cli_mode = %v, want true", p1["interactive_cli_mode"])
	}

	p2, ok := byID["icm-list-proj-2"]
	if !ok {
		t.Fatal("icm-list-proj-2 missing from list")
	}
	if val, ok := p2["interactive_cli_mode"].(bool); ok && val {
		t.Errorf("icm-list-proj-2: interactive_cli_mode = true, want false (default)")
	}
	if _, ok := p2["interactive_cli_mode"]; !ok {
		t.Error("icm-list-proj-2: interactive_cli_mode field absent, want false")
	}
}
