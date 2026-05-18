package spawner

import (
	"testing"
)

// TestMatchAnyCaseInsensitive verifies case-insensitive substring matching.
func TestMatchAnyCaseInsensitive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		text     string
		patterns []string
		wantOK   bool
		wantPat  string
	}{
		{"exact_match", "Rate limit exceeded", []string{"Rate limit exceeded"}, true, "Rate limit exceeded"},
		{"case_upper", "RATE LIMIT EXCEEDED", []string{"rate limit exceeded"}, true, "rate limit exceeded"},
		{"case_mixed", "rAtE LiMiT", []string{"Rate Limit"}, true, "Rate Limit"},
		{"no_match", "normal output", []string{"rate limit", "quota exceeded"}, false, ""},
		{"first_match_wins", "rate limit exceeded quota exceeded", []string{"rate limit exceeded", "quota exceeded"}, true, "rate limit exceeded"},
		{"empty_patterns", "some text", []string{}, false, ""},
		{"partial_match", "you've hit your limit today", []string{"hit your limit"}, true, "hit your limit"},
		{"empty_text", "", []string{"rate limit"}, false, ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pat, ok := matchAnyCaseInsensitive(tt.text, tt.patterns)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v (text=%q)", ok, tt.wantOK, tt.text)
			}
			if ok && pat != tt.wantPat {
				t.Errorf("pattern = %q, want %q", pat, tt.wantPat)
			}
		})
	}
}

// TestClaudeAdapter_ClassifyExit covers all Claude default patterns.
func TestClaudeAdapter_ClassifyExit(t *testing.T) {
	t.Parallel()
	a := &ClaudeAdapter{}
	tests := []struct {
		name       string
		recent     string
		stderr     string
		extraLimit []string
		extraError []string
		wantClass  RetryClass
		wantPat    string
	}{
		{
			name:      "rate_limit_hit_limit",
			recent:    "You've hit your limit for today. Please try again later.",
			wantClass: RetryClassRateLimit,
			wantPat:   "You've hit your limit",
		},
		{
			name:      "rate_limit_org_monthly",
			recent:    "You've hit your org's monthly usage limit on the API.",
			wantClass: RetryClassRateLimit,
			wantPat:   "You've hit your org's monthly usage limit",
		},
		{
			name:      "rate_limit_admin_disabled",
			recent:    "Your usage allocation has been disabled by your admin.",
			wantClass: RetryClassRateLimit,
			wantPat:   "Your usage allocation has been disabled by your admin",
		},
		{
			name:      "error_api_error",
			recent:    "API Error: 500 internal server error occurred",
			wantClass: RetryClassError,
			wantPat:   "API Error:",
		},
		{
			name:      "error_not_logged_in",
			stderr:    "Not logged in. Please authenticate first.",
			wantClass: RetryClassError,
			wantPat:   "Not logged in",
		},
		{
			name:      "error_nested_session",
			recent:    "cannot be launched inside another Claude Code session",
			wantClass: RetryClassError,
			wantPat:   "cannot be launched inside another Claude Code session",
		},
		{
			name:      "case_insensitive_rate_limit",
			recent:    "YOU'VE HIT YOUR LIMIT — please wait",
			wantClass: RetryClassRateLimit,
			wantPat:   "You've hit your limit",
		},
		{
			// Rate-limit check comes before error check.
			name:      "limit_priority_over_error",
			recent:    "API Error: You've hit your limit",
			wantClass: RetryClassRateLimit,
			wantPat:   "You've hit your limit",
		},
		{
			name:      "no_match_returns_none",
			recent:    "just some random output",
			stderr:    "exiting with code 1",
			wantClass: RetryClassNone,
		},
		{
			name:       "extra_limit_pattern_extends_defaults",
			recent:     "custom-quota-exhausted signal received",
			extraLimit: []string{"custom-quota-exhausted"},
			wantClass:  RetryClassRateLimit,
			wantPat:    "custom-quota-exhausted",
		},
		{
			name:       "extra_error_pattern_extends_defaults",
			recent:     "custom-provider-failure detected",
			extraError: []string{"custom-provider-failure"},
			wantClass:  RetryClassError,
			wantPat:    "custom-provider-failure",
		},
		{
			// Limit in stderr wins over error in recent text.
			name:      "limit_in_stderr_wins",
			recent:    "API Error: something bad",
			stderr:    "You've hit your limit",
			wantClass: RetryClassRateLimit,
			wantPat:   "You've hit your limit",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cls, pat := a.ClassifyExit(tt.recent, tt.stderr, 1, tt.extraLimit, tt.extraError)
			if cls != tt.wantClass {
				t.Errorf("class = %v, want %v (recent=%q stderr=%q)", cls, tt.wantClass, tt.recent, tt.stderr)
			}
			if tt.wantPat != "" && pat != tt.wantPat {
				t.Errorf("pattern = %q, want %q", pat, tt.wantPat)
			}
			if tt.wantClass == RetryClassNone && pat != "" {
				t.Errorf("pattern = %q, want empty for RetryClassNone", pat)
			}
		})
	}
}

// TestCodexAdapter_ClassifyExit covers Codex rate-limit defaults and absent error defaults.
func TestCodexAdapter_ClassifyExit(t *testing.T) {
	t.Parallel()
	a := &CodexAdapter{}
	tests := []struct {
		name       string
		recent     string
		extraError []string
		wantClass  RetryClass
	}{
		{"rate_limit_exceeded", "Rate limit exceeded for this endpoint", nil, RetryClassRateLimit},
		{"rate_limit_reached", "rate limit reached — please retry", nil, RetryClassRateLimit},
		{"429_too_many", "429 Too Many Requests", nil, RetryClassRateLimit},
		{"quota_exceeded", "quota exceeded for your plan", nil, RetryClassRateLimit},
		{"insufficient_quota", "insufficient_quota error encountered", nil, RetryClassRateLimit},
		{"usage_limit", "You've hit your usage limit", nil, RetryClassRateLimit},
		{"case_insensitive_quota", "QUOTA EXCEEDED please wait", nil, RetryClassRateLimit},
		// Codex has NO default error patterns.
		{"no_default_error_api_error", "API Error: 500 internal", nil, RetryClassNone},
		{"no_default_error_not_logged_in", "Not logged in", nil, RetryClassNone},
		// Extra error patterns are applied.
		{"extra_error_pattern", "custom-codex-failure", []string{"custom-codex-failure"}, RetryClassError},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cls, _ := a.ClassifyExit(tt.recent, "", 1, nil, tt.extraError)
			if cls != tt.wantClass {
				t.Errorf("class = %v, want %v (recent=%q)", cls, tt.wantClass, tt.recent)
			}
		})
	}
}

// TestOpencodeAdapter_ClassifyExit verifies empty defaults and user pattern extension.
func TestOpencodeAdapter_ClassifyExit(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	tests := []struct {
		name       string
		recent     string
		extraLimit []string
		extraError []string
		wantClass  RetryClass
	}{
		{"no_defaults_rate_limit_text", "Rate limit exceeded", nil, nil, RetryClassNone},
		{"no_defaults_error_text", "API Error: something", nil, nil, RetryClassNone},
		{"user_limit_pattern", "opencode-rate-limit", []string{"opencode-rate-limit"}, nil, RetryClassRateLimit},
		{"user_error_pattern", "opencode-provider-error", nil, []string{"opencode-provider-error"}, RetryClassError},
		{
			// Limit checked before error; both present → limit wins.
			name:       "limit_priority_over_error_user_patterns",
			recent:     "opencode-rate-limit opencode-provider-error",
			extraLimit: []string{"opencode-rate-limit"},
			extraError: []string{"opencode-provider-error"},
			wantClass:  RetryClassRateLimit,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cls, _ := a.ClassifyExit(tt.recent, "", 1, tt.extraLimit, tt.extraError)
			if cls != tt.wantClass {
				t.Errorf("class = %v, want %v (recent=%q)", cls, tt.wantClass, tt.recent)
			}
		})
	}
}

// TestGeminiAdapter_ClassifyExit verifies empty defaults and user pattern extension.
func TestGeminiAdapter_ClassifyExit(t *testing.T) {
	t.Parallel()
	a := &GeminiAdapter{}
	tests := []struct {
		name       string
		recent     string
		extraLimit []string
		extraError []string
		wantClass  RetryClass
	}{
		{"no_defaults", "Rate limit exceeded", nil, nil, RetryClassNone},
		{"user_limit", "gemini-quota-reached signal", []string{"gemini-quota-reached"}, nil, RetryClassRateLimit},
		{"user_error", "gemini-api-failure detected", nil, []string{"gemini-api-failure"}, RetryClassError},
		{"case_insensitive_user", "GEMINI-QUOTA-REACHED", []string{"gemini-quota-reached"}, nil, RetryClassRateLimit},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cls, _ := a.ClassifyExit(tt.recent, "", 1, tt.extraLimit, tt.extraError)
			if cls != tt.wantClass {
				t.Errorf("class = %v, want %v (recent=%q)", cls, tt.wantClass, tt.recent)
			}
		})
	}
}

// TestRetryClass_Constants verifies the enum values.
func TestRetryClass_Constants(t *testing.T) {
	t.Parallel()
	if RetryClassNone != 0 {
		t.Errorf("RetryClassNone = %d, want 0", RetryClassNone)
	}
	if RetryClassRateLimit != 1 {
		t.Errorf("RetryClassRateLimit = %d, want 1", RetryClassRateLimit)
	}
	if RetryClassError != 2 {
		t.Errorf("RetryClassError = %d, want 2", RetryClassError)
	}
}
