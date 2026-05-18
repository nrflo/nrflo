package spawner

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// makeSDKTestErr builds a *sdk.Error with the given status code for use in
// spawner-layer tests. Request and Response are populated so Error() won't panic.
// If errType is non-empty, UnmarshalJSON sets the internal error type.
func makeSDKTestErr(statusCode int, errType sdk.ErrorType) *sdk.Error {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	resp := &http.Response{StatusCode: statusCode}
	apiErr := &sdk.Error{
		StatusCode: statusCode,
		Request:    req,
		Response:   resp,
	}
	if errType != "" {
		body := `{"error":{"type":"` + string(errType) + `","message":"test error"}}`
		_ = json.Unmarshal([]byte(body), apiErr)
	}
	return apiErr
}

// immediateRateLimitProvider returns a typed 429 SDK error on the first Run call.
type immediateRateLimitProvider struct{}

func (p *immediateRateLimitProvider) Name() string                { return "rate_limit_test" }
func (p *immediateRateLimitProvider) MaxContext(model string) int { return 200000 }
func (p *immediateRateLimitProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	return nil, makeSDKTestErr(429, sdk.ErrorTypeRateLimitError)
}

// TestAPIBackend_RateLimitRetry_SetsStatusContinue verifies the api goroutine
// rate-limit dance: runner exits RATE_LIMITED → goroutine broadcasts, registers
// stop as continue/rate_limit, waits (instant with 0 backoff), sets CONTINUE.
// No DB pool is wired so all repo calls are no-ops.
func TestAPIBackend_RateLimitRetry_SetsStatusContinue(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{
		Clock:    clk,
		Provider: &immediateRateLimitProvider{},
		AgentSvc: &noopAgentSvc{},
		// No Pool: all DB operations become no-ops.
	})
	b := newAPIBackend(s)

	proc := &processInfo{
		sessionID:    "sess-rl-1",
		projectID:    "p1",
		ticketID:     "t1",
		workflowName: "wf",
		agentID:      "ag-rl-1",
		agentType:    "test-agent",
		modelID:      "api:claude",
		startTime:    clk.Now(),
		doneCh:       make(chan struct{}),
		maxContext:   1000,
		rateLimitConfig: rateLimitConfig{
			Enabled:        true,
			InitialBackoff: 0, // zero delay → Clock.After(0) fires immediately
			MaxWait:        time.Hour,
		},
	}
	prep := &prepResult{
		executionMode:    "api",
		apiInitialPrompt: "hi",
		apiMaxIterations: 1,
		apiMaxContext:    1000,
		apiDeadline:      time.Now().Add(5 * time.Second),
	}

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("doneCh not closed within 3s — goroutine may be stuck")
	}

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.rateLimitRetryCount != 1 {
		t.Errorf("rateLimitRetryCount = %d, want 1", proc.rateLimitRetryCount)
	}
	if proc.failRestartCount != 0 {
		t.Errorf("failRestartCount = %d, want 0 (rate-limit must not touch fail counter)", proc.failRestartCount)
	}
}

// TestAPIBackend_RateLimitRetry_DisabledConfig verifies that when rate-limit
// retry is disabled (Enabled=false), the goroutine takes the else branch and
// finalStatus stays RATE_LIMITED (mapped to continue/rate_limit by the else).
func TestAPIBackend_RateLimitRetry_DisabledConfig(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{
		Clock:    clk,
		Provider: &immediateRateLimitProvider{},
		AgentSvc: &noopAgentSvc{},
	})
	b := newAPIBackend(s)

	proc := &processInfo{
		sessionID:    "sess-rl-2",
		projectID:    "p1",
		ticketID:     "t1",
		workflowName: "wf",
		agentID:      "ag-rl-2",
		agentType:    "test-agent",
		modelID:      "api:claude",
		startTime:    clk.Now(),
		doneCh:       make(chan struct{}),
		maxContext:   1000,
		rateLimitConfig: rateLimitConfig{
			Enabled: false, // disabled → else branch
			MaxWait: time.Hour,
		},
	}
	prep := &prepResult{
		executionMode:    "api",
		apiInitialPrompt: "hi",
		apiMaxIterations: 1,
		apiMaxContext:    1000,
		apiDeadline:      time.Now().Add(5 * time.Second),
	}

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("doneCh not closed within 3s")
	}

	// When disabled, the goroutine calls mapFinalStatus("RATE_LIMITED") →
	// (continue, rate_limit) via the else branch, and finalStatus remains
	// RATE_LIMITED (unchanged by the else path — registerAgentStopWithReason
	// is called but finalStatus is not mutated again).
	if proc.finalStatus != "RATE_LIMITED" {
		t.Errorf("finalStatus = %q, want RATE_LIMITED (disabled config, no CONTINUE flip)", proc.finalStatus)
	}
	if proc.rateLimitRetryCount != 0 {
		t.Errorf("rateLimitRetryCount = %d, want 0 (disabled path must not increment)", proc.rateLimitRetryCount)
	}
}
