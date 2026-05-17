package socket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/service"
	"be/internal/ws"
)

// newArtifactHandlerEnv returns a handlerTestEnv with artifactSvc wired and NRFLO_HOME set.
func newArtifactHandlerEnv(t *testing.T) (*handlerTestEnv, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)
	env := newHandlerTestEnv(t)
	dataPath := filepath.Join(dir, "nrflo.data")
	env.handler.artifactSvc = service.NewArtifactService(env.pool, clock.Real(), env.hub, dataPath)
	return env, dir
}

// seedArtifactSession seeds a running agent session and returns (wfiID, sessionID).
func seedArtifactSession(t *testing.T, env *handlerTestEnv, suffix string) (wfiID, sessionID string) {
	t.Helper()
	ticketID := "ARTTIX-" + suffix
	env.createTicketAndWorkflow(t, ticketID)
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)`,
		env.project, ticketID).Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}
	sessionID = "artsess-" + suffix
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test', 'test', 'model', 'running', datetime('now'), datetime('now'))`,
		sessionID, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return wfiID, sessionID
}

func TestArtifactAdd_ValidationErrors(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)

	cases := []struct {
		name   string
		params string
	}{
		{"missing_session_id", `{"name":"f","content_b64":"aGk="}`},
		{"missing_name", `{"session_id":"s","content_b64":"aGk="}`},
		{"missing_content", `{"session_id":"s","name":"f"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{ID: "r", Method: "artifact.add", Params: json.RawMessage(tc.params)}
			resp := env.handler.Handle(req)
			if resp.Error == nil {
				t.Fatal("expected validation error")
			}
			if resp.Error.Code != ErrCodeValidation {
				t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeValidation)
			}
		})
	}
}

func TestArtifactAdd_UnknownSession(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	params := json.RawMessage(`{"session_id":"no-such-sess","name":"f.txt","content_b64":"aGk="}`)
	resp := env.handler.Handle(Request{ID: "r", Method: "artifact.add", Params: params})
	if resp.Error == nil {
		t.Fatal("expected not-found error")
	}
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeNotFound)
	}
}

func TestArtifactAdd_OversizeContent(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	_, sessionID := seedArtifactSession(t, env, "big")

	// 33 MiB of zeros base64-encoded triggers the 32 MiB cap.
	// 33 * 1024 * 1024 / 3 * 4 = 44040192 base64 chars
	bigData := make([]byte, 33*1024*1024)
	bigEncoded := base64.StdEncoding.EncodeToString(bigData)

	paramsMap := map[string]string{
		"session_id":  sessionID,
		"name":        "huge.bin",
		"content_b64": bigEncoded,
	}
	paramsJSON, _ := json.Marshal(paramsMap)
	resp := env.handler.Handle(Request{ID: "r", Method: "artifact.add", Params: paramsJSON})
	if resp.Error == nil {
		t.Fatal("expected error for oversize artifact")
	}
	if !strings.Contains(resp.Error.Message, "32 MiB") {
		t.Errorf("error message = %q, want mention of 32 MiB", resp.Error.Message)
	}
}

func TestArtifactAdd_HappyPath(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	wfiID, sessionID := seedArtifactSession(t, env, "add1")

	// Subscribe a test WS client to project-wide events BEFORE calling the handler.
	wsClient, recvCh := ws.NewTestClient(env.hub, "ws-add-test")
	env.hub.Subscribe(wsClient, env.project, "")

	content := []byte("artifact content data")
	encoded := base64.StdEncoding.EncodeToString(content)
	paramsMap := map[string]string{
		"session_id":   sessionID,
		"name":         "out.txt",
		"content_b64":  encoded,
		"content_type": "text/plain",
	}
	paramsJSON, _ := json.Marshal(paramsMap)
	resp := env.handler.Handle(Request{ID: "r", Method: "artifact.add", Params: paramsJSON})

	if resp.Error != nil {
		t.Fatalf("artifact.add error: %v", resp.Error)
	}

	// Verify response fields
	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Name != "out.txt" {
		t.Errorf("name = %q, want out.txt", result.Name)
	}

	// Verify DB row: source=agent, created_by_session=sessionID
	artifacts, err := env.handler.artifactSvc.List(context.Background(), wfiID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	a := artifacts[0]
	if a.Source != model.ArtifactSourceAgent {
		t.Errorf("source = %q, want %q", a.Source, model.ArtifactSourceAgent)
	}
	if a.CreatedBySession != sessionID {
		t.Errorf("created_by_session = %q, want %q", a.CreatedBySession, sessionID)
	}

	// Verify EventArtifactCreated broadcast was sent
	select {
	case msg := <-recvCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventArtifactCreated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventArtifactCreated)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for EventArtifactCreated broadcast")
	}
}

func TestArtifactList_ValidationErrors(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)

	resp := env.handler.Handle(Request{ID: "r", Method: "artifact.list", Params: json.RawMessage(`{}`)})
	if resp.Error == nil {
		t.Fatal("expected validation error")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
}

func TestArtifactList_HappyPath(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	wfiID, sessionID := seedArtifactSession(t, env, "list1")
	_ = wfiID

	content := []byte("list test")
	encoded := base64.StdEncoding.EncodeToString(content)
	paramsMap := map[string]string{"session_id": sessionID, "name": "listed.txt", "content_b64": encoded}
	paramsJSON, _ := json.Marshal(paramsMap)
	env.handler.Handle(Request{ID: "r1", Method: "artifact.add", Params: paramsJSON})

	listParams := json.RawMessage(`{"session_id":"` + sessionID + `"}`)
	resp := env.handler.Handle(Request{ID: "r2", Method: "artifact.list", Params: listParams})
	if resp.Error != nil {
		t.Fatalf("artifact.list error: %v", resp.Error)
	}

	var items []struct {
		Name      string `json:"name"`
		SizeBytes int64  `json:"size_bytes"`
		Source    string `json:"source"`
	}
	if err := json.Unmarshal(resp.Result, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Name != "listed.txt" {
		t.Errorf("name = %q, want listed.txt", items[0].Name)
	}
	if items[0].Source != model.ArtifactSourceAgent {
		t.Errorf("source = %q, want agent", items[0].Source)
	}
}

func TestArtifactGet_ValidationErrors(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)

	cases := []struct{ name, params string }{
		{"missing_session", `{"name":"f"}`},
		{"missing_name", `{"session_id":"s"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.handler.Handle(Request{ID: "r", Method: "artifact.get", Params: json.RawMessage(tc.params)})
			if resp.Error == nil {
				t.Fatal("expected validation error")
			}
			if resp.Error.Code != ErrCodeValidation {
				t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeValidation)
			}
		})
	}
}

func TestArtifactGet_UnknownName(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	_, sessionID := seedArtifactSession(t, env, "getn")

	paramsJSON, _ := json.Marshal(map[string]string{"session_id": sessionID, "name": "nosuchfile.txt"})
	resp := env.handler.Handle(Request{ID: "r", Method: "artifact.get", Params: paramsJSON})
	if resp.Error == nil {
		t.Fatal("expected not-found error")
	}
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeNotFound)
	}
}

func TestArtifactGet_HappyPath(t *testing.T) {
	env, _ := newArtifactHandlerEnv(t)
	_, sessionID := seedArtifactSession(t, env, "geth")

	// Create artifact via add
	content := []byte("get test content")
	encoded := base64.StdEncoding.EncodeToString(content)
	addParams, _ := json.Marshal(map[string]string{
		"session_id": sessionID, "name": "get.txt", "content_b64": encoded,
	})
	addResp := env.handler.Handle(Request{ID: "r1", Method: "artifact.add", Params: addParams})
	if addResp.Error != nil {
		t.Fatalf("artifact.add: %v", addResp.Error)
	}

	getParams, _ := json.Marshal(map[string]string{"session_id": sessionID, "name": "get.txt"})
	getResp := env.handler.Handle(Request{ID: "r2", Method: "artifact.get", Params: getParams})
	if getResp.Error != nil {
		t.Fatalf("artifact.get: %v", getResp.Error)
	}

	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(getResp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Path == "" {
		t.Fatal("path is empty")
	}
	if !filepath.IsAbs(result.Path) {
		t.Errorf("path not absolute: %q", result.Path)
	}
	got, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", got, content)
	}
}
