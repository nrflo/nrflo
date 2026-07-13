package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// shrinkDynamicWorkflowPoll shrinks the dynamicWorkflow poll vars for the
// duration of the test, restoring the previous values on cleanup. Mirrors how
// newTestExternalClient shrinks deepResearchPollInterval/deepResearchMaxWait,
// but dynamicWorkflow has its own vars so tests must shrink them separately.
func shrinkDynamicWorkflowPoll(t *testing.T, interval, maxWait time.Duration) {
	t.Helper()
	prevInterval, prevWait := dynamicWorkflowPollInterval, dynamicWorkflowMaxWait
	dynamicWorkflowPollInterval = interval
	dynamicWorkflowMaxWait = maxWait
	t.Cleanup(func() { dynamicWorkflowPollInterval, dynamicWorkflowMaxWait = prevInterval, prevWait })
}

func TestDispatchExternalMCP_ToolsList_IncludesDynamicWorkflowTools(t *testing.T) {
	resp := dispatchExternalMCP(context.Background(), makeMCPReq(1, "tools/list", ""), &nrfloHTTPClient{defaultProject: "p1"})
	res := resp.Result.(map[string]interface{})
	tools := res["tools"].([]map[string]interface{})
	want := map[string]bool{
		"deep_research":    false,
		"run_workflow":     false,
		"get_workflow":     false,
		"list_workflows":   false,
		"dynamic_workflow": false,
		"revise_plan":      false,
		"approve_plan":     false,
	}
	for _, tl := range tools {
		want[tl["name"].(string)] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tools/list missing %q", name)
		}
	}
}

func TestExternalTool_DynamicWorkflow_HappyPath(t *testing.T) {
	var starts, gets, plans int
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dynamic-workflow"):
			starts++
			_, _ = w.Write([]byte(`{"instance_id":"inst-1"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			if got := r.URL.Query().Get("instance_id"); got != "inst-1" {
				t.Errorf("instance_id query = %q, want inst-1", got)
			}
			gets++
			if gets < 3 {
				_, _ = w.Write([]byte(`{"state":{"status":"planning"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"state":{"status":"waiting_approval","plan":{"latest_revision":1}}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan"):
			plans++
			_, _ = w.Write([]byte(`{"head":{"latest_revision":1},"manifest":{"goal":"build X","layers":[]}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, time.Millisecond, 5*time.Second)

	out, err := callExternalTool(context.Background(), c, "dynamic_workflow", json.RawMessage(`{"instructions":"build X"}`))
	if err != nil {
		t.Fatalf("dynamic_workflow: %v", err)
	}
	if !strings.Contains(out, `"status": "waiting_approval"`) || !strings.Contains(out, `"plan"`) {
		t.Errorf("output missing waiting_approval state/plan: %q", out)
	}
	// A caller parked at the boundary must see the manifest it is being asked to
	// revise/approve, merged in alongside the revision it must pin to.
	if plans != 1 {
		t.Errorf("plan fetches = %d, want 1", plans)
	}
	if !strings.Contains(out, `"manifest"`) || !strings.Contains(out, `"latest_revision": 1`) {
		t.Errorf("output missing merged plan draft (manifest + latest_revision): %q", out)
	}
	if starts != 1 {
		t.Errorf("starts = %d, want 1", starts)
	}
	if gets < 3 {
		t.Errorf("gets = %d, want it to have blocked through >=2 planning polls", gets)
	}
}

func TestExternalTool_DynamicWorkflow_Validation(t *testing.T) {
	c := &nrfloHTTPClient{defaultProject: "p1"}
	if _, err := callExternalTool(context.Background(), c, "dynamic_workflow", json.RawMessage(`{}`)); err == nil {
		t.Error("dynamic_workflow without instructions should error")
	}
	if _, err := callExternalTool(context.Background(), c, "revise_plan", json.RawMessage(`{"revision":1}`)); err == nil {
		t.Error("revise_plan without instance_id should error")
	}
	if _, err := callExternalTool(context.Background(), c, "approve_plan", json.RawMessage(`{"revision":1}`)); err == nil {
		t.Error("approve_plan without instance_id should error")
	}
}

func TestExternalTool_DynamicWorkflow_PassesModeThrough(t *testing.T) {
	var gotBody map[string]any
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dynamic-workflow"):
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"instance_id":"inst-mode"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			_, _ = w.Write([]byte(`{"state":{"status":"waiting_approval"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan"):
			_, _ = w.Write([]byte(`{"head":{"latest_revision":1},"manifest":{"goal":"g"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, time.Millisecond, 5*time.Second)

	if _, err := callExternalTool(context.Background(), c, "dynamic_workflow",
		json.RawMessage(`{"instructions":"build X","mode":"auto"}`)); err != nil {
		t.Fatalf("dynamic_workflow: %v", err)
	}
	if gotBody["mode"] != "auto" {
		t.Errorf("mode = %v, want %q", gotBody["mode"], "auto")
	}
}

func TestExternalTool_DynamicWorkflow_OmitsEmptyMode(t *testing.T) {
	hasKey := true
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dynamic-workflow"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, hasKey = body["mode"]
			_, _ = w.Write([]byte(`{"instance_id":"inst-nomode"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			_, _ = w.Write([]byte(`{"state":{"status":"waiting_approval"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan"):
			_, _ = w.Write([]byte(`{"head":{"latest_revision":1},"manifest":{"goal":"g"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, time.Millisecond, 5*time.Second)

	if _, err := callExternalTool(context.Background(), c, "dynamic_workflow",
		json.RawMessage(`{"instructions":"build X"}`)); err != nil {
		t.Fatalf("dynamic_workflow: %v", err)
	}
	if hasKey {
		t.Error("mode should be omitted from the request body when not supplied")
	}
}

func TestExternalTool_DynamicWorkflow_PerCallProjectOverridesDefault(t *testing.T) {
	var sawHeaderProject string
	var sawPathProject bool
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dynamic-workflow"):
			sawHeaderProject = r.Header.Get("X-Project")
			sawPathProject = strings.Contains(r.URL.Path, "/projects/p2/dynamic-workflow")
			_, _ = w.Write([]byte(`{"instance_id":"inst-proj"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			_, _ = w.Write([]byte(`{"state":{"status":"waiting_approval"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan"):
			_, _ = w.Write([]byte(`{"head":{"latest_revision":1},"manifest":{"goal":"g"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, time.Millisecond, 5*time.Second)

	// defaultProject is "p1" (set by newTestExternalClient); the per-call arg must win.
	if _, err := callExternalTool(context.Background(), c, "dynamic_workflow",
		json.RawMessage(`{"instructions":"build X","project":"p2"}`)); err != nil {
		t.Fatalf("dynamic_workflow: %v", err)
	}
	if sawHeaderProject != "p2" {
		t.Errorf("X-Project = %q, want p2 (per-call override)", sawHeaderProject)
	}
	if !sawPathProject {
		t.Error("expected the dynamic-workflow POST path to be scoped under /projects/p2/")
	}
}
