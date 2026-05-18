package apirun

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider/mock"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// TestRunner_RateLimitError_SetsStatusAndSkipsErrorSvc verifies that when the
// provider returns a rate-limit SDK error, the runner sets RATE_LIMITED status,
// emits a system message with "rate_limit", and does NOT call ErrorSvc.
func TestRunner_RateLimitError_SetsStatusAndSkipsErrorSvc(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}

	// Use StatusCode=200 with rate_limit_error type so the test exercises the
	// Type()-based detection path, not the StatusCode fallback.
	prov := mock.New(mock.Script{Err: makeSDKErr(200, sdk.ErrorTypeRateLimitError)})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		ErrorSvc:      errSvc,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "RATE_LIMITED" {
		t.Errorf("FinalStatus = %q, want RATE_LIMITED", proc.FinalStatus())
	}

	// ErrorSvc must NOT be called for rate-limit errors.
	if errs := errSvc.Errors(); len(errs) != 0 {
		t.Errorf("ErrorSvc called %d times, want 0; calls = %+v", len(errs), errs)
	}

	// Sink should have a system message containing "rate_limit".
	found := false
	for _, c := range sink.Calls() {
		if c.category == "system" && strings.Contains(c.content, "rate_limit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected system message with 'rate_limit', got %+v", sink.Calls())
	}
}

// TestRunner_RateLimitError_StatusCode429_SkipsErrorSvc is the StatusCode-fallback
// variant: same expectations when the SDK error has no explicit type but 429 code.
func TestRunner_RateLimitError_StatusCode429_SkipsErrorSvc(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}

	prov := mock.New(mock.Script{Err: makeSDKErr(429, "")})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		ErrorSvc:      errSvc,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "RATE_LIMITED" {
		t.Errorf("FinalStatus = %q, want RATE_LIMITED", proc.FinalStatus())
	}
	if errs := errSvc.Errors(); len(errs) != 0 {
		t.Errorf("ErrorSvc called %d times, want 0", len(errs))
	}
}

// TestRunner_AuthError_CallsErrorSvc verifies that a 401 auth error sets FAIL
// status and DOES call ErrorSvc (unlike rate-limit errors).
func TestRunner_AuthError_CallsErrorSvc(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}

	prov := mock.New(mock.Script{Err: makeSDKErr(401, "")})

	r := NewRunner(Config{
		Provider:      prov,
		Sink:          sink,
		ErrorSvc:      errSvc,
		InitialPrompt: "hi",
		MaxIterations: 3,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})
	proc := newTestProc()
	r.Run(context.Background(), proc)

	if proc.FinalStatus() != "FAIL" {
		t.Errorf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	if errs := errSvc.Errors(); len(errs) != 1 {
		t.Errorf("ErrorSvc called %d times, want 1", len(errs))
	}
}
