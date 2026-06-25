package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/service"
)

// newTestExternalClient returns a client pointed at h, with fast polling.
func newTestExternalClient(t *testing.T, h http.HandlerFunc) *nrfloHTTPClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	prevInterval, prevWait := deepResearchPollInterval, deepResearchMaxWait
	deepResearchPollInterval = time.Millisecond
	deepResearchMaxWait = 5 * time.Second
	t.Cleanup(func() { deepResearchPollInterval, deepResearchMaxWait = prevInterval, prevWait })
	c := &nrfloHTTPClient{base: srv.URL, token: "tok", defaultProject: "p1", hc: srv.Client()}
	c.cwdResolved = true // disable cwd auto-detect here; exercised in *_cwd_test.go
	return c
}

func TestDispatchExternalMCP_Initialize(t *testing.T) {
	resp := dispatchExternalMCP(makeMCPReq(1, "initialize", ""), &nrfloHTTPClient{defaultProject: "p1"})
	res, ok := resp.Result.(map[string]interface{})
	if !ok || res["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected initialize result: %+v", resp)
	}
}

func TestDispatchExternalMCP_ToolsList(t *testing.T) {
	resp := dispatchExternalMCP(makeMCPReq(1, "tools/list", ""), &nrfloHTTPClient{defaultProject: "p1"})
	res := resp.Result.(map[string]interface{})
	tools := res["tools"].([]map[string]interface{})
	want := map[string]bool{"deep_research": false, "run_workflow": false, "get_workflow": false, "list_workflows": false}
	for _, tl := range tools {
		want[tl["name"].(string)] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tools/list missing %q", name)
		}
	}
}

func TestExternalTool_DeepResearch_HappyPath(t *testing.T) {
	var runs, gets int
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Project") != "p1" {
			t.Errorf("missing auth/project headers: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflow/run"):
			runs++
			_, _ = w.Write([]byte(`{"instance_id":"inst-1"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			gets++
			if gets < 2 {
				_, _ = w.Write([]byte(`{"state":{"status":"running"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"state":{"status":"completed","workflow_findings":{"report":"FINAL REPORT"}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})

	out, err := callExternalTool(c, "deep_research", json.RawMessage(`{"question":"what is X"}`))
	if err != nil {
		t.Fatalf("deep_research: %v", err)
	}
	if out != "FINAL REPORT" {
		t.Errorf("report = %q, want FINAL REPORT", out)
	}
	if runs != 1 || gets < 2 {
		t.Errorf("runs=%d gets=%d, want 1 run + >=2 polls", runs, gets)
	}
}

func TestExternalTool_DeepResearch_Failed(t *testing.T) {
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"instance_id":"inst-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"state":{"status":"failed"}}`))
	})
	_, err := callExternalTool(c, "deep_research", json.RawMessage(`{"question":"q"}`))
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("want failure error, got %v", err)
	}
}

func TestExternalTool_RunAndGetAndList(t *testing.T) {
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflow/run"):
			_, _ = w.Write([]byte(`{"instance_id":"inst-9"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			if got := r.URL.Query().Get("instance_id"); got != "inst-9" {
				t.Errorf("instance_id query = %q", got)
			}
			_, _ = w.Write([]byte(`{"state":{"status":"running","current_phase":"L1"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/api/v1/workflows"):
			_, _ = w.Write([]byte(`{"deep-research":{"is_global":true}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	})

	run, err := callExternalTool(c, "run_workflow", json.RawMessage(`{"workflow":"deep-research","instructions":"go"}`))
	if err != nil || !strings.Contains(run, `"instance_id":"inst-9"`) {
		t.Fatalf("run_workflow = %q err=%v", run, err)
	}
	get, err := callExternalTool(c, "get_workflow", json.RawMessage(`{"instance_id":"inst-9"}`))
	if err != nil || !strings.Contains(get, `"current_phase": "L1"`) {
		t.Fatalf("get_workflow = %q err=%v", get, err)
	}
	list, err := callExternalTool(c, "list_workflows", nil)
	if err != nil || !strings.Contains(list, "deep-research") {
		t.Fatalf("list_workflows = %q err=%v", list, err)
	}
}

func TestExternalTool_Validation(t *testing.T) {
	c := &nrfloHTTPClient{defaultProject: "p1"}
	if _, err := callExternalTool(c, "deep_research", json.RawMessage(`{}`)); err == nil {
		t.Error("deep_research without question should error")
	}
	if _, err := callExternalTool(c, "run_workflow", json.RawMessage(`{}`)); err == nil {
		t.Error("run_workflow without workflow should error")
	}
	if _, err := callExternalTool(c, "nope", nil); err == nil {
		t.Error("unknown tool should error")
	}
}

func TestExternalTool_PerCallProjectOverridesDefault(t *testing.T) {
	var sawProject string
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		sawProject = r.Header.Get("X-Project")
		_, _ = w.Write([]byte(`{"deep-research":{}}`))
	})
	// defaultProject is "p1"; the per-call arg must win.
	if _, err := callExternalTool(c, "list_workflows", json.RawMessage(`{"project":"p2"}`)); err != nil {
		t.Fatalf("list_workflows: %v", err)
	}
	if sawProject != "p2" {
		t.Errorf("X-Project = %q, want p2 (per-call override)", sawProject)
	}
}

func TestResolveProject_FallsBackToGlobal(t *testing.T) {
	// No arg, no cwd match (cwdResolved pre-set, empty), no NRFLO_PROJECT default →
	// the hidden global project is the final fallback (never errors).
	c := &nrfloHTTPClient{}
	c.cwdResolved = true
	if got := c.resolveProject(context.Background(), ""); got != service.GlobalProjectID {
		t.Fatalf("resolveProject fallback = %q, want %q", got, service.GlobalProjectID)
	}
	// Explicit arg and NRFLO_PROJECT default still win over the global fallback.
	if got := c.resolveProject(context.Background(), "explicit"); got != "explicit" {
		t.Errorf("arg should win: got %q", got)
	}
	c2 := &nrfloHTTPClient{defaultProject: "p1"}
	c2.cwdResolved = true
	if got := c2.resolveProject(context.Background(), ""); got != "p1" {
		t.Errorf("NRFLO_PROJECT default should win over global: got %q", got)
	}
}

func TestExternalTool_ServerError_IsToolError(t *testing.T) {
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
	resp := dispatchExternalMCP(makeMCPReq(7, "tools/call", `{"name":"list_workflows"}`), c)
	res := resp.Result.(map[string]interface{})
	if res["isError"] != true {
		t.Fatalf("expected isError=true on 403, got %+v", res)
	}
}
