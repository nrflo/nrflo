package cli

// Leaf dispatch tests for the observer cobra subtree.
// Each test spins up a fake Unix socket server and verifies that the correct
// JSON-RPC method name and session_id reach the socket for every one of the
// 20 observer methods.
//
// Tests run sequentially (no t.Parallel) — env vars are process-global.

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

type observerCapture struct {
	method string
	params map[string]interface{}
}

// startFakeObserverSocket creates a temporary Unix socket server that accepts
// connections, records incoming JSON-RPC requests, and responds with a minimal
// success payload.  The channel has capacity 30 so all 20 dispatch subtests
// can buffer without blocking.
func startFakeObserverSocket(t *testing.T) (sockPath string, reqs <-chan observerCapture) {
	t.Helper()
	sockPath = filepath.Join(t.TempDir(), "obs.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("startFakeObserverSocket: %v", err)
	}
	ch := make(chan observerCapture, 30)
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go fakeObserverHandle(conn, ch)
		}
	}()
	return sockPath, ch
}

// fakeObserverHandle reads one JSON-RPC request line from conn, sends it to
// ch, and writes back a minimal success response.  Connections that close
// immediately (e.g. IsServerRunning probes) are silently ignored.
func fakeObserverHandle(conn net.Conn, ch chan<- observerCapture) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var wireReq struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &wireReq); err != nil || wireReq.ID == "" {
			return
		}
		var params map[string]interface{}
		_ = json.Unmarshal(wireReq.Params, &params)
		if params == nil {
			params = map[string]interface{}{}
		}
		ch <- observerCapture{method: wireReq.Method, params: params}
		resp, _ := json.Marshal(struct {
			ID string `json:"id"`
		}{ID: wireReq.ID})
		conn.Write(append(resp, '\n')) //nolint:errcheck
	}
}

// TestObserverLeafDispatch exercises every leaf RunE in the observer subtree
// against a fake socket server and verifies:
//  1. The method name dispatched over the socket matches the expected string.
//  2. The session_id param is populated from NRF_SESSION_ID.
func TestObserverLeafDispatch(t *testing.T) {
	sockPath, reqs := startFakeObserverSocket(t)
	t.Setenv("NRFLO_SOCKET", sockPath)
	t.Setenv("NRF_SESSION_ID", "dispatch-test-sess")

	// Pre-set ProjectID for all workflow/project-scope subtests and restore at end.
	origPID := ProjectID
	ProjectID = "dispatch-proj"
	t.Cleanup(func() { ProjectID = origPID })

	type leafCase struct {
		name       string
		scope      string
		run        func() error
		wantMethod string
	}

	cases := []leafCase{
		{"workflow.show", "workflow",
			func() error { return obsWorkflowShowCmd.RunE(nil, nil) },
			"observer.workflow.show"},
		{"workflow.runs", "workflow",
			func() error { return obsWorkflowRunsCmd.RunE(nil, nil) },
			"observer.workflow.runs"},
		{"workflow.findings", "workflow",
			func() error { return obsWorkflowFindingsCmd.RunE(nil, nil) },
			"observer.workflow.findings"},
		{"workflow.logs", "workflow",
			func() error { return obsWorkflowLogsCmd.RunE(nil, nil) },
			"observer.workflow.logs"},
		{"workflow.trigger", "workflow",
			func() error { return obsWorkflowTriggerCmd.RunE(nil, nil) },
			"observer.workflow.trigger"},
		{"workflow.retry_failed", "workflow",
			func() error { return obsWorkflowRetryFailedCmd.RunE(nil, nil) },
			"observer.workflow.retry_failed"},
		{"workflow.def.update", "workflow",
			func() error { return obsWorkflowDefUpdateCmd.RunE(nil, nil) },
			"observer.workflow.def.update"},
		{"project.workflows", "project",
			func() error { return obsProjectWorkflowsCmd.RunE(nil, nil) },
			"observer.project.workflows"},
		{"project.runs", "project",
			func() error { return obsProjectRunsCmd.RunE(nil, nil) },
			"observer.project.runs"},
		{"project.findings", "project",
			func() error { return obsProjectFindingsCmd.RunE(nil, nil) },
			"observer.project.findings"},
		{"project.env.list", "project",
			func() error { return obsProjectEnvListCmd.RunE(nil, nil) },
			"observer.project.env.list"},
		{"project.env.set", "project",
			func() error { return obsProjectEnvSetCmd.RunE(nil, nil) },
			"observer.project.env.set"},
		{"project.env.unset", "project",
			func() error { return obsProjectEnvUnsetCmd.RunE(nil, nil) },
			"observer.project.env.unset"},
		{"project.workflow.create", "project",
			func() error { return obsProjectWFCreateCmd.RunE(nil, nil) },
			"observer.project.workflow.create"},
		{"project.workflow.delete", "project",
			func() error { return obsProjectWFDeleteCmd.RunE(nil, nil) },
			"observer.project.workflow.delete"},
		{"global.projects", "global",
			func() error { return obsGlobalProjectsCmd.RunE(nil, nil) },
			"observer.global.projects"},
		{"global.recent_sessions", "global",
			func() error { return obsGlobalRecentSessionsCmd.RunE(nil, nil) },
			"observer.global.recent_sessions"},
		{"global.health", "global",
			func() error { return obsGlobalHealthCmd.RunE(nil, nil) },
			"observer.global.health"},
		{"global.project.create", "global",
			func() error { return obsGlobalProjectCreateCmd.RunE(nil, nil) },
			"observer.global.project.create"},
		{"global.project.delete", "global",
			func() error { return obsGlobalProjectDeleteCmd.RunE(nil, nil) },
			"observer.global.project.delete"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NRF_OBSERVER", "1")
			t.Setenv("NRF_OBSERVER_SCOPE", tc.scope)

			if err := tc.run(); err != nil {
				t.Fatalf("RunE(%q): %v", tc.name, err)
			}

			timer := time.NewTimer(2 * time.Second)
			select {
			case got := <-reqs:
				timer.Stop()
				if got.method != tc.wantMethod {
					t.Errorf("method=%q want %q", got.method, tc.wantMethod)
				}
				if sessID, _ := got.params["session_id"].(string); sessID != "dispatch-test-sess" {
					t.Errorf("session_id=%q want %q", sessID, "dispatch-test-sess")
				}
			case <-timer.C:
				t.Fatalf("timeout: no request captured for %q", tc.name)
			}
		})
	}
}
