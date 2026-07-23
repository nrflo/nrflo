package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleListSystemAgentRuns_MalformedSince400 verifies a non-RFC3339
// since value returns 400.
func TestHandleListSystemAgentRuns_MalformedSince400(t *testing.T) {
	s := newSystemAgentRunsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs?since=not-a-date", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleListSystemAgentRuns_LimitClamping verifies limit is clamped to
// [1, 200], with an out-of-range or non-numeric value falling back sanely.
func TestHandleListSystemAgentRuns_LimitClamping(t *testing.T) {
	s := newSystemAgentRunsServer(t)

	cases := []struct {
		query     string
		wantLimit int
	}{
		{"limit=0", 1},
		{"limit=-5", 1},
		{"limit=500", 200},
		{"limit=abc", 50}, // non-numeric ignored, default retained
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs?"+tc.query, nil)
			rr := httptest.NewRecorder()
			s.handleListSystemAgentRuns(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
			}
			_, limit := decodeRunsResponse(t, rr)
			if limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tc.wantLimit)
			}
		})
	}
}

// TestHandleListSystemAgentRuns_EmptyResultItemsNotNil verifies an empty
// result serializes items as [] rather than null.
func TestHandleListSystemAgentRuns_EmptyResultItemsNotNil(t *testing.T) {
	s := newSystemAgentRunsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !containsLiteralEmptyItems(rr.Body.String()) {
		t.Errorf("body = %s, want items:[] (not null)", rr.Body.String())
	}
}

// TestSystemAgentRuns_AdminOnly_ServiceTokenForbidden verifies the real route
// (admin-gated) rejects a service-token bearer with 403, since requireAdmin
// is human-admin-only.
func TestSystemAgentRuns_AdminOnly_ServiceTokenForbidden(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-runs-auth", "ci")

	// The real route wraps this handler with admin() (requireAdmin), which
	// never populates a user context for a bearer request — assert directly
	// via requireAdmin, mirroring the route's actual gating.
	adminChain := s.sessionMgr.LoadAndSave(s.requireAdmin(http.HandlerFunc(s.handleListSystemAgentRuns)))
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	req2.Header.Set("Authorization", "Bearer "+plain)
	rr2 := httptest.NewRecorder()
	adminChain.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("requireAdmin with service-token bearer status = %d, want 403", rr2.Code)
	}
}

// TestSystemAgentRuns_AdminOnly_HumanAdminAllowed verifies a logged-in human
// admin can reach the real registered route end-to-end.
func TestSystemAgentRuns_AdminOnly_HumanAdminAllowed(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	resp, err := as.client.Get(as.baseURL + "/api/v1/system-agent-runs")
	if err != nil {
		t.Fatalf("GET /system-agent-runs: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin GET status = %d, want 200", resp.StatusCode)
	}
}
