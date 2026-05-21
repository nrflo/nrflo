package cli

// Tests for be/internal/cli/agent_consult.go
// Uses the fake-Unix-socket dispatch pattern from observer_dispatch_test.go.
// Tests run sequentially (no t.Parallel) — env vars and cobra stdin are process-global.

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type consultCapture struct {
	method string
	params map[string]interface{}
}

// startFakeConsultSocket starts a fake Unix socket that captures JSON-RPC requests
// and responds with {"answer": answer}.  Uses /tmp to stay within the macOS
// sun_path limit (104 bytes).
func startFakeConsultSocket(t *testing.T, answer string) (string, <-chan consultCapture) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "consulttest")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "c.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	ch := make(chan consultCapture, 5)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go fakeConsultHandle(conn, ch, answer)
		}
	}()
	return sockPath, ch
}

// fakeConsultHandle reads one JSON-RPC request, sends it to ch, and responds with
// {"answer": answer}.  Connections that close immediately (IsServerRunning probes)
// are silently ignored.
func fakeConsultHandle(conn net.Conn, ch chan<- consultCapture, answer string) {
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
		ch <- consultCapture{method: wireReq.Method, params: params}
		result, _ := json.Marshal(map[string]string{"answer": answer})
		resp, _ := json.Marshal(struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
		}{ID: wireReq.ID, Result: result})
		conn.Write(append(resp, '\n')) //nolint:errcheck
	}
}

// resetConsultFlags resets package-level flag vars and cobra stdin between tests.
func resetConsultFlags() {
	agentConsultConsultant = ""
	agentConsultQuestion = ""
	agentConsultJSON = false
	agentConsultCmd.SetIn(nil)
}

// setupConsultEnv sets env vars and package state for agent consult tests.
func setupConsultEnv(t *testing.T, sockPath string) {
	t.Helper()
	t.Setenv("NRFLO_SOCKET", sockPath)
	t.Setenv("NRFLO_PROJECT", "proj-1")
	t.Setenv("NRF_SESSION_ID", "sess-abc")
	t.Setenv("NRF_WORKFLOW_INSTANCE_ID", "wfi-xyz")
	origPID := ProjectID
	ProjectID = "proj-1"
	t.Cleanup(func() { ProjectID = origPID })
	t.Cleanup(resetConsultFlags)
}

// awaitConsultReq waits up to 2 s for a request on ch.
func awaitConsultReq(t *testing.T, ch <-chan consultCapture) consultCapture {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	select {
	case got := <-ch:
		timer.Stop()
		return got
	case <-timer.C:
		t.Fatalf("timeout: no request captured")
		return consultCapture{}
	}
}

// TestAgentConsultCmd_HappyPath verifies full socket dispatch: correct method,
// params (consultant, question, session_id, instance_id), and stdout answer.
func TestAgentConsultCmd_HappyPath(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "42 is the answer")
	setupConsultEnv(t, sockPath)
	agentConsultConsultant = "my-consultant"
	agentConsultQuestion = "what is the answer?"

	out := captureStdout(t, func() {
		if err := agentConsultCmd.RunE(agentConsultCmd, nil); err != nil {
			t.Errorf("RunE: %v", err)
		}
	})

	got := awaitConsultReq(t, reqs)

	if got.method != "agent.consult" {
		t.Errorf("method=%q want %q", got.method, "agent.consult")
	}
	if c, _ := got.params["consultant"].(string); c != "my-consultant" {
		t.Errorf("consultant param=%q want %q", c, "my-consultant")
	}
	if q, _ := got.params["question"].(string); q != "what is the answer?" {
		t.Errorf("question param=%q want %q", q, "what is the answer?")
	}
	if s, _ := got.params["session_id"].(string); s != "sess-abc" {
		t.Errorf("session_id param=%q want %q", s, "sess-abc")
	}
	if i, _ := got.params["instance_id"].(string); i != "wfi-xyz" {
		t.Errorf("instance_id param=%q want %q", i, "wfi-xyz")
	}
	if !strings.Contains(out, "42 is the answer") {
		t.Errorf("stdout=%q want it to contain the answer", out)
	}
}

// TestAgentConsultCmd_JSONFlag verifies that --json emits {"answer":"..."} JSON.
func TestAgentConsultCmd_JSONFlag(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "the json answer")
	setupConsultEnv(t, sockPath)
	agentConsultConsultant = "my-consultant"
	agentConsultQuestion = "what is the json answer?"
	agentConsultJSON = true

	out := captureStdout(t, func() {
		if err := agentConsultCmd.RunE(agentConsultCmd, nil); err != nil {
			t.Errorf("RunE: %v", err)
		}
	})
	awaitConsultReq(t, reqs)

	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("stdout not valid JSON: %v\noutput=%q", err, out)
	}
	if result["answer"] != "the json answer" {
		t.Errorf("answer=%q want %q", result["answer"], "the json answer")
	}
}

// TestAgentConsultCmd_MissingSessionID verifies that absent NRF_SESSION_ID returns
// an error mentioning NRF_SESSION_ID without dispatching a socket call.
func TestAgentConsultCmd_MissingSessionID(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "")
	setupConsultEnv(t, sockPath)
	t.Setenv("NRF_SESSION_ID", "") // override to empty
	agentConsultConsultant = "my-consultant"
	agentConsultQuestion = "any question"

	err := agentConsultCmd.RunE(agentConsultCmd, nil)
	if err == nil {
		t.Fatal("expected error for missing NRF_SESSION_ID, got nil")
	}
	if !strings.Contains(err.Error(), "NRF_SESSION_ID") {
		t.Errorf("error=%q should mention NRF_SESSION_ID", err.Error())
	}
	select {
	case <-reqs:
		t.Error("unexpected socket request after NRF_SESSION_ID validation failure")
	default:
	}
}

// TestAgentConsultCmd_EmptyConsultant verifies that empty --consultant returns an
// error without dispatching a socket call.
func TestAgentConsultCmd_EmptyConsultant(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "")
	setupConsultEnv(t, sockPath)
	agentConsultConsultant = "" // empty
	agentConsultQuestion = "some question"

	err := agentConsultCmd.RunE(agentConsultCmd, nil)
	if err == nil {
		t.Fatal("expected error for empty --consultant, got nil")
	}
	if !strings.Contains(err.Error(), "--consultant") {
		t.Errorf("error=%q should mention --consultant", err.Error())
	}
	select {
	case <-reqs:
		t.Error("unexpected socket request after --consultant validation failure")
	default:
	}
}

// TestAgentConsultCmd_EmptyQuestionAndStdin verifies that an empty --question and
// empty stdin return an error without dispatching a socket call.
func TestAgentConsultCmd_EmptyQuestionAndStdin(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "")
	setupConsultEnv(t, sockPath)
	agentConsultConsultant = "some-consultant"
	agentConsultQuestion = "" // triggers stdin read
	agentConsultCmd.SetIn(strings.NewReader(""))

	err := agentConsultCmd.RunE(agentConsultCmd, nil)
	if err == nil {
		t.Fatal("expected error for empty question + empty stdin, got nil")
	}
	if !strings.Contains(err.Error(), "--question") {
		t.Errorf("error=%q should mention --question", err.Error())
	}
	select {
	case <-reqs:
		t.Error("unexpected socket request after empty question validation failure")
	default:
	}
}

// TestAgentConsultCmd_QuestionFromStdin verifies that setting --question "-" causes
// the question to be read from stdin.
func TestAgentConsultCmd_QuestionFromStdin(t *testing.T) {
	sockPath, reqs := startFakeConsultSocket(t, "stdin answer")
	setupConsultEnv(t, sockPath)
	agentConsultConsultant = "consultant-x"
	agentConsultQuestion = "-"
	agentConsultCmd.SetIn(strings.NewReader("question from stdin"))

	captureStdout(t, func() {
		if err := agentConsultCmd.RunE(agentConsultCmd, nil); err != nil {
			t.Errorf("RunE: %v", err)
		}
	})

	got := awaitConsultReq(t, reqs)
	if q, _ := got.params["question"].(string); q != "question from stdin" {
		t.Errorf("question param=%q want %q", q, "question from stdin")
	}
}

// TestAgentConsultCmd_Registered verifies that "consult" is registered as a
// subcommand of agentCmd.
func TestAgentConsultCmd_Registered(t *testing.T) {
	t.Parallel()
	if !contains(getCommandNames(agentCmd), "consult") {
		t.Error("agentCmd missing subcommand \"consult\"")
	}
}
