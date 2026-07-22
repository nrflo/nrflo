package apirun

import (
	"context"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider/mock"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// SetProviderHardFail/ProviderHardFail implement the fakeProc (runner_test.go)
// half of apirun.ProcState's tier-fallback signal, kept in this file (not
// runner_test.go) to stay under the file-size ratchet.
func (p *fakeProc) SetProviderHardFail() {
	p.mu.Lock()
	p.providerHardFail = true
	p.mu.Unlock()
}
func (p *fakeProc) ProviderHardFail() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.providerHardFail
}

// TestRunner_AuthError_SetsProviderHardFail verifies that a 401 auth error
// (RetryClassError) calls proc.SetProviderHardFail() — the signal the
// spawner's tier-fallback engine consumes via shouldAdvanceChain — in
// addition to the existing FAIL status + ErrorSvc call already covered by
// TestRunner_AuthError_CallsErrorSvc.
func TestRunner_AuthError_SetsProviderHardFail(t *testing.T) {
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
		t.Fatalf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	if !proc.ProviderHardFail() {
		t.Error("ProviderHardFail() = false, want true (RetryClassError must call SetProviderHardFail)")
	}
}

// TestRunner_ServerError_SetsProviderHardFail verifies a 5xx provider error
// (also RetryClassError) sets the hard-fail flag identically to a 401.
func TestRunner_ServerError_SetsProviderHardFail(t *testing.T) {
	sink := &recordingSink{}
	errSvc := &recordingErrSvc{}

	prov := mock.New(mock.Script{Err: makeSDKErr(500, "")})

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
		t.Fatalf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	if !proc.ProviderHardFail() {
		t.Error("ProviderHardFail() = false, want true (500 is RetryClassError)")
	}
}

// TestRunner_RateLimitError_NeverSetsProviderHardFail is the required guard:
// rate-limit/overload errors (RetryClassRateLimit) must NEVER call
// SetProviderHardFail — the tier-fallback chain must never advance on a
// transient rate limit, which stays in-band via the existing retry dance.
func TestRunner_RateLimitError_NeverSetsProviderHardFail(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "sdk_rate_limit_type", err: makeSDKErr(200, sdk.ErrorTypeRateLimitError)},
		{name: "sdk_overloaded_type", err: makeSDKErr(200, sdk.ErrorTypeOverloadedError)},
		{name: "http_429", err: makeSDKErr(429, "")},
		{name: "http_529_overloaded", err: makeSDKErr(529, "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &recordingSink{}
			errSvc := &recordingErrSvc{}
			prov := mock.New(mock.Script{Err: tc.err})

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
				t.Fatalf("FinalStatus = %q, want RATE_LIMITED", proc.FinalStatus())
			}
			if proc.ProviderHardFail() {
				t.Error("ProviderHardFail() = true, want false (rate-limit must stay in-band, never advance the chain)")
			}
		})
	}
}
