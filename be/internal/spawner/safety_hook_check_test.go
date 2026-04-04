package spawner

import (
	"os/exec"
	"testing"
)

// skipIfJqMissing skips the test if jq is not installed on the host.
func skipIfJqMissing(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed — skipping CheckSafetyHook execution test")
	}
}

func TestCheckSafetyHook_EmptyCommand(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{Enabled: true, AllowGit: true}
	_, _, err := CheckSafetyHook(cfg, "")
	if err == nil {
		t.Error("expected error for empty command, got nil")
	}
}

func TestCheckSafetyHook_AllowedCommand(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         true,
		RmRfAllowedPaths: []string{"/tmp"},
	}
	allowed, reason, err := CheckSafetyHook(cfg, "ls -la")
	if err != nil {
		t.Fatalf("CheckSafetyHook(ls -la) error: %v", err)
	}
	if !allowed {
		t.Errorf("expected allowed=true for 'ls -la', got blocked: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason for allowed command, got: %s", reason)
	}
}

func TestCheckSafetyHook_BlockedHardcodedPattern(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:  true,
		AllowGit: true,
	}
	// "rm -rf /" is a hardcoded blocked pattern
	allowed, reason, err := CheckSafetyHook(cfg, "rm -rf /")
	if err != nil {
		t.Fatalf("CheckSafetyHook(rm -rf /) error: %v", err)
	}
	if allowed {
		t.Error("expected allowed=false for 'rm -rf /', got allowed=true")
	}
	if reason == "" {
		t.Error("expected non-empty reason for blocked command")
	}
}

func TestCheckSafetyHook_BlockedUserDefinedPattern(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:           true,
		AllowGit:          true,
		RmRfAllowedPaths:  []string{"/tmp"},
		DangerousPatterns: []string{"DROP TABLE"},
	}
	allowed, reason, err := CheckSafetyHook(cfg, "DROP TABLE users")
	if err != nil {
		t.Fatalf("CheckSafetyHook(DROP TABLE users) error: %v", err)
	}
	if allowed {
		t.Errorf("expected allowed=false for user-defined pattern, got allowed=true")
	}
	if reason == "" {
		t.Error("expected non-empty reason for blocked user pattern")
	}
}

func TestCheckSafetyHook_BlockedGitOpWhenAllowGitFalse(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         false,
		RmRfAllowedPaths: []string{"/tmp"},
	}
	allowed, reason, err := CheckSafetyHook(cfg, "git push origin main")
	if err != nil {
		t.Fatalf("CheckSafetyHook(git push origin main) error: %v", err)
	}
	if allowed {
		t.Errorf("expected allowed=false for 'git push' when allow_git=false, got allowed=true")
	}
	if reason == "" {
		t.Error("expected non-empty reason for blocked git op")
	}
}

func TestCheckSafetyHook_AllowedGitOpWhenAllowGitTrue(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         true,
		RmRfAllowedPaths: []string{"/tmp"},
	}
	allowed, reason, err := CheckSafetyHook(cfg, "git push origin main")
	if err != nil {
		t.Fatalf("CheckSafetyHook(git push origin main, allow_git=true) error: %v", err)
	}
	if !allowed {
		t.Errorf("expected allowed=true for 'git push' when allow_git=true, got blocked: %s", reason)
	}
}

func TestCheckSafetyHook_AllowedRmOnAllowedPath(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         true,
		RmRfAllowedPaths: []string{"node_modules", "/tmp"},
	}
	cases := []struct {
		cmd     string
		allowed bool
	}{
		{"rm -rf node_modules", true},
		{"rm -rf dist", false},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			allowed, reason, err := CheckSafetyHook(cfg, tc.cmd)
			if err != nil {
				t.Fatalf("CheckSafetyHook(%q) error: %v", tc.cmd, err)
			}
			if tc.allowed && !allowed {
				t.Errorf("CheckSafetyHook(%q) = blocked (%s), want allowed", tc.cmd, reason)
			}
			if !tc.allowed && allowed {
				t.Errorf("CheckSafetyHook(%q) = allowed, want blocked", tc.cmd)
			}
		})
	}
}

func TestCheckSafetyHook_BlockedRmOnNonAllowedPath(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         true,
		RmRfAllowedPaths: []string{"node_modules"},
	}
	// dist is not in the allowed paths
	allowed, reason, err := CheckSafetyHook(cfg, "rm -rf dist")
	if err != nil {
		t.Fatalf("CheckSafetyHook(rm -rf dist) error: %v", err)
	}
	if allowed {
		t.Errorf("expected allowed=false for 'rm -rf dist' with no matching allowed path, got allowed=true")
	}
	if reason == "" {
		t.Error("expected non-empty reason for blocked rm")
	}
}

func TestCheckSafetyHook_BlockedRmWhenNoAllowedPaths(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:  true,
		AllowGit: true,
		// No RmRfAllowedPaths: all rm -rf blocked
	}
	allowed, reason, err := CheckSafetyHook(cfg, "rm -rf build")
	if err != nil {
		t.Fatalf("CheckSafetyHook(rm -rf build) error: %v", err)
	}
	if allowed {
		t.Errorf("expected allowed=false when no allowed paths configured, got allowed=true")
	}
	if reason == "" {
		t.Error("expected non-empty reason for blocked rm")
	}
}

func TestCheckSafetyHook_HardcodedPatternsVariants(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:  true,
		AllowGit: true,
	}
	// All four hardcoded dangerous rm patterns should be blocked
	cases := []string{
		"rm -rf /",
		"rm -rf ~",
		"rm -rf .",
		"rm -rf ..",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			allowed, reason, err := CheckSafetyHook(cfg, cmd)
			if err != nil {
				t.Fatalf("CheckSafetyHook(%q) error: %v", cmd, err)
			}
			if allowed {
				t.Errorf("CheckSafetyHook(%q) = allowed=true, want blocked", cmd)
			}
			if reason == "" {
				t.Errorf("CheckSafetyHook(%q) returned empty reason for blocked command", cmd)
			}
		})
	}
}

func TestCheckSafetyHook_CommandWithSpecialJSONChars(t *testing.T) {
	skipIfJqMissing(t)
	// Command containing special JSON chars — should not cause a marshal/parse error
	cfg := SafetyHookConfig{
		Enabled:          true,
		AllowGit:         true,
		RmRfAllowedPaths: []string{"/tmp"},
	}
	// Backslash and double-quote in command — must be properly JSON-escaped
	allowed, _, err := CheckSafetyHook(cfg, `echo "hello world"`)
	if err != nil {
		t.Fatalf("CheckSafetyHook with special chars error: %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true for echo with quoted arg")
	}
}

func TestCheckSafetyHook_MultipleUserPatterns(t *testing.T) {
	skipIfJqMissing(t)
	cfg := SafetyHookConfig{
		Enabled:           true,
		AllowGit:          true,
		RmRfAllowedPaths:  []string{"/tmp"},
		DangerousPatterns: []string{"DANGEROUS_QUERY", "evalmalicious", "foo|bar"},
	}
	cases := []struct {
		cmd     string
		blocked bool
	}{
		{"DANGEROUS_QUERY users", true},
		{"evalmalicious payload", true},
		{"something foo|bar baz", true},  // pipe in pattern matched literally
		{"ls /tmp", false},               // allowed — no dangerous pattern matches
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			allowed, reason, err := CheckSafetyHook(cfg, tc.cmd)
			if err != nil {
				t.Fatalf("CheckSafetyHook(%q) error: %v", tc.cmd, err)
			}
			if tc.blocked && allowed {
				t.Errorf("CheckSafetyHook(%q) = allowed=true, want blocked", tc.cmd)
			}
			if !tc.blocked && !allowed {
				t.Errorf("CheckSafetyHook(%q) = blocked (%s), want allowed", tc.cmd, reason)
			}
		})
	}
}
