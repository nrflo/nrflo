package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

type handlerValidationCase struct {
	name string
	// endpoint keys into the endpoints map; if empty the case runs against every endpoint.
	endpoint    string
	seedProject bool
	project     string            // X-Project header value ("" = omit header)
	jsonBody    map[string]string // sent when useRaw is false
	rawBody     string            // raw (possibly malformed) body; used when useRaw is true
	useRaw      bool
	wantCode    int
	wantErr     string // exact match against {"error": ...}; "" skips the check
}

// TestWorkflowControlHandlers_Validation consolidates the request-validation
// cases (missing project / workflow / session_id, malformed/empty JSON body)
// for the ticket-scoped take-control, exit-interactive, restart, and
// retry-failed endpoints. Each case boots a full API server, so they are
// table-driven to share that cost. HappyPath / not-found / not-failed flows
// live in the per-endpoint test files.
func TestWorkflowControlHandlers_Validation(t *testing.T) {
	const (
		errMissingProject  = "X-Project header or project query param required"
		errMissingWorkflow = "workflow name is required"
		errMissingSession  = "session_id is required"
	)

	// endpoints maps a logical name to the ticket-scoped URL suffix.
	endpoints := map[string]string{
		"take-control":     "take-control",
		"exit-interactive": "exit-interactive",
		"restart":          "restart",
		"retry-failed":     "retry-failed",
	}

	cases := []handlerValidationCase{
		{
			name:        "MissingProject",
			seedProject: false,
			project:     "",
			jsonBody:    map[string]string{"workflow": "test", "session_id": "sess-1"},
			wantCode:    http.StatusBadRequest,
			wantErr:     errMissingProject,
		},
		{
			name:        "MissingWorkflow",
			seedProject: true,
			project:     "proj",
			jsonBody:    map[string]string{"workflow": "", "session_id": "sess-1"},
			wantCode:    http.StatusBadRequest,
			wantErr:     errMissingWorkflow,
		},
		{
			name:        "MissingSessionID",
			seedProject: true,
			project:     "proj",
			jsonBody:    map[string]string{"workflow": "test", "session_id": ""},
			wantCode:    http.StatusBadRequest,
			wantErr:     errMissingSession,
		},
		{name: "InvalidJSON", endpoint: "restart", seedProject: true, project: "proj", rawBody: "{invalid json", useRaw: true, wantCode: http.StatusBadRequest},
		{name: "InvalidJSON", endpoint: "retry-failed", seedProject: true, project: "proj", rawBody: "{invalid json", useRaw: true, wantCode: http.StatusBadRequest},
		{name: "InvalidJSON", endpoint: "take-control", seedProject: true, project: "proj", rawBody: "{not valid json", useRaw: true, wantCode: http.StatusBadRequest},
		{name: "EmptyBody", endpoint: "restart", seedProject: true, project: "proj", rawBody: "", useRaw: true, wantCode: http.StatusBadRequest},
		{name: "EmptyBody", endpoint: "retry-failed", seedProject: true, project: "proj", rawBody: "", useRaw: true, wantCode: http.StatusBadRequest},
	}

	for _, c := range cases {
		c := c
		targets := map[string]string{}
		if c.endpoint != "" {
			targets[c.endpoint] = endpoints[c.endpoint]
		} else {
			targets = endpoints
		}
		for epName, epSuffix := range targets {
			epName, epSuffix := epName, epSuffix
			t.Run(epName+"/"+c.name, func(t *testing.T) {
				runHandlerValidationCase(t, epSuffix, c)
			})
		}
	}
}

func runHandlerValidationCase(t *testing.T, endpoint string, c handlerValidationCase) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}
	if c.seedProject {
		seedProject(t, dbPath, "proj")
	}
	baseURL, client := startAPIServer(t, dbPath)

	var bodyReader io.Reader
	if c.useRaw {
		bodyReader = bytes.NewBufferString(c.rawBody)
	} else {
		b, _ := json.Marshal(c.jsonBody)
		bodyReader = bytes.NewBuffer(b)
	}

	req, _ := http.NewRequest("POST",
		baseURL+"/api/v1/tickets/TICK-1/workflow/"+endpoint, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if c.project != "" {
		req.Header.Set("X-Project", c.project)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != c.wantCode {
		t.Fatalf("expected %d, got %d: %s", c.wantCode, resp.StatusCode, string(respBody))
	}
	if c.wantErr != "" {
		var errResp map[string]string
		json.Unmarshal(respBody, &errResp)
		if errResp["error"] != c.wantErr {
			t.Fatalf("unexpected error: %q, want %q", errResp["error"], c.wantErr)
		}
	}
}
