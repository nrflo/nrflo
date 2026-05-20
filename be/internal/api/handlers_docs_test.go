package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetAgentManual(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantStatus int
		wantTitle  string
		wantError  string
	}{
		{name: "common kind", query: "?kind=common", wantStatus: http.StatusOK, wantTitle: "Common"},
		{name: "cli kind", query: "?kind=cli", wantStatus: http.StatusOK, wantTitle: "CLI Agents"},
		{name: "python kind", query: "?kind=python", wantStatus: http.StatusOK, wantTitle: "Python Agents"},
		{name: "api kind", query: "?kind=api", wantStatus: http.StatusOK, wantTitle: "API Agents"},
		{name: "no kind defaults to common", query: "", wantStatus: http.StatusOK, wantTitle: "Common"},
		{name: "unknown kind returns 400", query: "?kind=bogus", wantStatus: http.StatusBadRequest, wantError: "unknown kind"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual"+tc.query, nil)
			rr := httptest.NewRecorder()

			server.handleGetAgentManual(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}

			ct := rr.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if tc.wantError != "" {
				got, _ := resp["error"].(string)
				if got != tc.wantError {
					t.Errorf("error = %q, want %q", got, tc.wantError)
				}
				return
			}

			title, ok := resp["title"].(string)
			if !ok || title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}

			content, ok := resp["content"].(string)
			if !ok || content == "" {
				t.Errorf("content field missing or empty for kind %q", tc.query)
			}
		})
	}
}
