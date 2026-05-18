package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func createPythonScriptTool(t *testing.T, s *Server, projectID, name string) {
	t.Helper()
	body := `{"name":"` + name + `","kind":"tool","tool_description":"does something useful"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createPythonScriptTool(%q) status = %d, want 201; body: %s", name, rr.Code, rr.Body.String())
	}
}

func TestHandleListPythonScripts_KindFilter_NoFilter_ReturnsMixed(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	createPythonScript(t, s, projectID, "Agent One", "print(1)")
	createPythonScriptTool(t, s, projectID, "Tool Alpha")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID, nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	list := decodePythonScriptList(t, rr)
	if len(list) != 2 {
		t.Errorf("len = %d, want 2 (1 agent + 1 tool)", len(list))
	}
}

func TestHandleListPythonScripts_KindFilter_Agent(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	createPythonScript(t, s, projectID, "Agent One", "print(1)")
	createPythonScriptTool(t, s, projectID, "Tool Alpha")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID+"&kind=agent", nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	list := decodePythonScriptList(t, rr)
	if len(list) != 1 {
		t.Errorf("len = %d, want 1 agent", len(list))
	}
	if len(list) > 0 && list[0].Kind != "agent" {
		t.Errorf("Kind = %q, want agent", list[0].Kind)
	}
}

func TestHandleListPythonScripts_KindFilter_Tool(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	createPythonScript(t, s, projectID, "Agent One", "print(1)")
	createPythonScriptTool(t, s, projectID, "Tool Alpha")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID+"&kind=tool", nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	list := decodePythonScriptList(t, rr)
	if len(list) != 1 {
		t.Errorf("len = %d, want 1 tool", len(list))
	}
	if len(list) > 0 && list[0].Kind != "tool" {
		t.Errorf("Kind = %q, want tool", list[0].Kind)
	}
}

func TestHandleListPythonScripts_KindFilter_Invalid(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts?project="+projectID+"&kind=garbage", nil)
	rr := httptest.NewRecorder()
	s.handleListPythonScripts(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "kind")
}

func TestHandleCreatePythonScript_KindTool_MissingDescription(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"My Tool","kind":"tool"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "tool_description")
}

func TestHandleCreatePythonScript_KindTool_InvalidSchema(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"Schema Tool","kind":"tool","tool_description":"does x","input_schema":"{\"type\":\"not-a-type\"}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "input_schema")
}

func TestHandleCreatePythonScript_KindTool_Success(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"My Tool","kind":"tool","tool_description":"Searches the web","input_schema":"{\"type\":\"object\"}","timeout_sec":45}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	script := decodePythonScript(t, rr)
	if script.Kind != "tool" {
		t.Errorf("Kind = %q, want tool", script.Kind)
	}
	if script.ToolDescription != "Searches the web" {
		t.Errorf("ToolDescription = %q", script.ToolDescription)
	}
	if script.TimeoutSec != 45 {
		t.Errorf("TimeoutSec = %d, want 45", script.TimeoutSec)
	}
	if script.ID == "" {
		t.Error("ID is empty")
	}
}

func TestHandleCreatePythonScript_KindAgent_Explicit(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"Explicit Agent","kind":"agent","code":"print(1)"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	script := decodePythonScript(t, rr)
	if script.Kind != "agent" {
		t.Errorf("Kind = %q, want agent", script.Kind)
	}
}

func TestHandleCreatePythonScript_KindInvalid(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"Bad Kind","kind":"wizard"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "kind")
}

func TestHandleCreatePythonScript_KindTool_TimeoutOutOfRange(t *testing.T) {
	s, projectID := newPythonScriptServer(t)

	body := `{"name":"Timeout Tool","kind":"tool","tool_description":"x","timeout_sec":601}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/python-scripts?project="+projectID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreatePythonScript(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "timeout_sec")
}
