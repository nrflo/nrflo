package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestExternalTool_RevisePlan_HappyPath(t *testing.T) {
	var gotBody map[string]any
	var sawPath string
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plan/revise") {
			sawPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"revision":2,"status":"waiting_approval"}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
	})

	out, err := callExternalTool(context.Background(), c, "revise_plan",
		json.RawMessage(`{"instance_id":"inst-7","revision":1,"plan":{"version":1,"goal":"g"},"feedback":"fb","answers":[{"question_id":"q1","answer":"a1"}]}`))
	if err != nil {
		t.Fatalf("revise_plan: %v", err)
	}
	if !strings.Contains(sawPath, "/workflow-instances/inst-7/plan/revise") {
		t.Errorf("path = %q, want it scoped to instance inst-7", sawPath)
	}
	if rev, _ := gotBody["revision"].(float64); rev != 1 {
		t.Errorf("revision = %v, want 1", gotBody["revision"])
	}
	manifest, ok := gotBody["manifest"].(map[string]any)
	if !ok || manifest["goal"] != "g" {
		t.Errorf("manifest (renamed from plan arg) = %v", gotBody["manifest"])
	}
	if gotBody["feedback"] != "fb" {
		t.Errorf("feedback = %v, want fb", gotBody["feedback"])
	}
	answers, ok := gotBody["answers"].([]any)
	if !ok || len(answers) != 1 {
		t.Errorf("answers = %v, want a single-element array", gotBody["answers"])
	}
	if !strings.Contains(out, `"status": "waiting_approval"`) {
		t.Errorf("output = %q", out)
	}
}

func TestExternalTool_RevisePlan_Validation(t *testing.T) {
	c := &nrfloHTTPClient{defaultProject: "p1"}
	if _, err := callExternalTool(context.Background(), c, "revise_plan", json.RawMessage(`{"revision":1}`)); err == nil {
		t.Error("revise_plan without instance_id should error")
	}
}

func TestExternalTool_ApprovePlan_HappyPath(t *testing.T) {
	var gotBody map[string]any
	var sawPath string
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plan/approve") {
			sawPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"revision":3,"status":"running"}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
	})

	out, err := callExternalTool(context.Background(), c, "approve_plan",
		json.RawMessage(`{"instance_id":"inst-8","revision":3}`))
	if err != nil {
		t.Fatalf("approve_plan: %v", err)
	}
	if !strings.Contains(sawPath, "/workflow-instances/inst-8/plan/approve") {
		t.Errorf("path = %q, want it scoped to instance inst-8", sawPath)
	}
	if len(gotBody) != 1 {
		t.Errorf("body = %v, want exactly {revision: 3}", gotBody)
	}
	if rev, _ := gotBody["revision"].(float64); rev != 3 {
		t.Errorf("revision = %v, want 3", gotBody["revision"])
	}
	if !strings.Contains(out, `"revision": 3`) {
		t.Errorf("output = %q", out)
	}
}

func TestExternalTool_ApprovePlan_Validation(t *testing.T) {
	c := &nrfloHTTPClient{defaultProject: "p1"}
	if _, err := callExternalTool(context.Background(), c, "approve_plan", json.RawMessage(`{"revision":1}`)); err == nil {
		t.Error("approve_plan without instance_id should error")
	}
}

func TestPollPlanState_CancelStopsWorkflow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var stopped bool
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflow/stop"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["instance_id"] == "inst-cancel" {
				stopped = true
			}
			_, _ = w.Write([]byte(`{"status":"stopping"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			cancel() // caller cancels mid-poll
			_, _ = w.Write([]byte(`{"state":{"status":"planning"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, time.Millisecond, 5*time.Second)

	if _, err := c.pollPlanState(ctx, "p1", "inst-cancel"); err == nil {
		t.Fatal("expected a cancellation error")
	}
	if !stopped {
		t.Error("expected workflow/stop to be called for the in-flight instance on cancellation")
	}
}

func TestPollPlanState_Timeout(t *testing.T) {
	c := newTestExternalClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflow/stop"):
			_, _ = w.Write([]byte(`{"status":"stopping"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			_, _ = w.Write([]byte(`{"state":{"status":"running"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	})
	shrinkDynamicWorkflowPoll(t, 2*time.Millisecond, 10*time.Millisecond)

	_, err := c.pollPlanState(context.Background(), "p1", "inst-timeout")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "inst-timeout") || !strings.Contains(err.Error(), "running") {
		t.Errorf("error = %q, want it to mention the instance id and its still-running status", err.Error())
	}
}
