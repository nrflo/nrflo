package refinery

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"
)

// TestFoldAutonomous_RejectsDegenerateOutput drives foldAutonomous
// synchronously (no sidecar goroutine) to verify that empty text and
// degenerate stop reasons (refusal, max_tokens) are rejected: the slot
// digest and as.nextFoldSeq must not advance.
func TestFoldAutonomous_RejectsDegenerateOutput(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-reject", "proj-auto-reject"
	wfiID, nodeID := "wfi-reject", "node-reject"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "work one", "work two")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("valid digest v1")
	stubBuildProvider(t, prov)

	as := mgr.newAutonomousSession(wfiID, nodeID, "")

	mgr.foldAutonomous(context.Background(), as, sessionID, projectID)

	s := getSlot(t, mgr, wfiID, nodeID)
	if s == nil {
		t.Fatal("slot after valid fold = nil, want a digest row")
	}
	if s.Content != "valid digest v1" {
		t.Errorf("Content after valid fold = %q, want %q", s.Content, "valid digest v1")
	}
	if s.Version != 1 {
		t.Errorf("Version after valid fold = %d, want 1", s.Version)
	}
	if got := nextFoldSeq(as); got != 2 {
		t.Errorf("nextFoldSeq after valid fold = %d, want 2", got)
	}

	seedMessages(t, pool, clk, sessionID, "delta to be rejected")

	cases := []struct {
		name     string
		response provider.FinalResponse
	}{
		{
			name: "empty text",
			response: provider.FinalResponse{
				StopReason: "end_turn",
				Content:    []provider.ContentBlock{{Type: "text", Text: "   "}},
			},
		},
		{
			name: "refusal",
			response: provider.FinalResponse{
				StopReason: "refusal",
				Content:    []provider.ContentBlock{{Type: "text", Text: "I can't help with that."}},
			},
		},
		{
			name: "max_tokens",
			response: provider.FinalResponse{
				StopReason: "max_tokens",
				Content:    []provider.ContentBlock{{Type: "text", Text: "partial truncated..."}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov.mu.Lock()
			prov.response = tc.response
			prov.mu.Unlock()

			mgr.foldAutonomous(context.Background(), as, sessionID, projectID)

			s := getSlot(t, mgr, wfiID, nodeID)
			if s == nil {
				t.Fatal("slot after rejected fold = nil, want unchanged v1 digest")
			}
			if s.Content != "valid digest v1" {
				t.Errorf("Content after %s = %q, want unchanged %q", tc.name, s.Content, "valid digest v1")
			}
			if s.Version != 1 {
				t.Errorf("Version after %s = %d, want unchanged 1", tc.name, s.Version)
			}
			if got := nextFoldSeq(as); got != 2 {
				t.Errorf("nextFoldSeq after %s = %d, want unchanged 2 (rejected fold must not advance progress)", tc.name, got)
			}
		})
	}
}

// TestFold_RejectsEmptyOutput_ConsolePath covers the console fold path
// (fold, not foldAutonomous): a valid fold followed by an empty-text
// response must leave the digest content/version unchanged.
func TestFold_RejectsEmptyOutput_ConsolePath(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-fold-reject", "proj-fold-reject"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("valid console digest")
	stubBuildProvider(t, prov)

	foldConsoleOnce(context.Background(), mgr, sessionID, projectID, []string{"event one"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get after valid fold: %v", err)
	}
	if d == nil || d.Content != "valid console digest" {
		t.Fatalf("Get after valid fold = %+v, want content %q", d, "valid console digest")
	}
	if d.Version != 1 {
		t.Fatalf("Version after valid fold = %d, want 1", d.Version)
	}

	prov.mu.Lock()
	prov.response = provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: ""}},
	}
	prov.mu.Unlock()

	foldConsoleOnce(context.Background(), mgr, sessionID, projectID, []string{"event two"})

	d2, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get after rejected fold: %v", err)
	}
	if d2 == nil || d2.Content != "valid console digest" {
		t.Errorf("Get after rejected fold = %+v, want unchanged content %q", d2, "valid console digest")
	}
	if d2.Version != 1 {
		t.Errorf("Version after rejected fold = %d, want unchanged 1", d2.Version)
	}
}

func TestIsDegenerateStopReason(t *testing.T) {
	cases := []struct {
		sr   string
		want bool
	}{
		{"max_tokens", true},
		{"refusal", true},
		{"end_turn", false},
		{"stop_sequence", false},
		{"tool_use", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.sr, func(t *testing.T) {
			if got := isDegenerateStopReason(tc.sr); got != tc.want {
				t.Errorf("isDegenerateStopReason(%q) = %v, want %v", tc.sr, got, tc.want)
			}
		})
	}
}

func nextFoldSeq(as *autonomousSession) int {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.nextFoldSeq
}
