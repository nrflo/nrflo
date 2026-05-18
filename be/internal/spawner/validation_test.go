package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// TestBuildValidationEnv verifies that session credentials are stripped
// and other env vars (including NRFLO_PROJECT) are preserved.
func TestBuildValidationEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		env     []string
		wantIn  []string
		wantOut []string
	}{
		{
			name:    "strips_agent_token",
			env:     []string{"NRFLO_AGENT_TOKEN=secret", "NRFLO_PROJECT=proj1"},
			wantIn:  []string{"NRFLO_PROJECT=proj1"},
			wantOut: []string{"NRFLO_AGENT_TOKEN=secret"},
		},
		{
			name:    "strips_session_id",
			env:     []string{"NRF_SESSION_ID=sess123", "NRFLO_PROJECT=proj1"},
			wantIn:  []string{"NRFLO_PROJECT=proj1"},
			wantOut: []string{"NRF_SESSION_ID=sess123"},
		},
		{
			name:    "strips_both_credentials",
			env:     []string{"NRFLO_AGENT_TOKEN=tok", "NRF_SESSION_ID=sid", "NRFLO_PROJECT=p", "PATH=/bin"},
			wantIn:  []string{"NRFLO_PROJECT=p", "PATH=/bin"},
			wantOut: []string{"NRFLO_AGENT_TOKEN=tok", "NRF_SESSION_ID=sid"},
		},
		{
			name: "empty_env",
			env:  []string{},
		},
		{
			name: "nil_env",
			env:  nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proc := &processInfo{env: tt.env}
			got := buildValidationEnv(proc)

			for _, want := range tt.wantIn {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildValidationEnv() missing %q in result %v", want, got)
				}
			}
			for _, notWant := range tt.wantOut {
				for _, g := range got {
					if g == notWant {
						t.Errorf("buildValidationEnv() should NOT contain %q", notWant)
					}
				}
			}
		})
	}
}

// TestRunOneValidationCommand covers exit codes and output capture.
func TestRunOneValidationCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cmd      string
		wantCode int
		wantOut  string
	}{
		{"exit0", "true", 0, ""},
		{"exit1", "false", 1, ""},
		{"exit_code_7", "exit 7", 7, ""},
		{"stdout_captured", "echo hello", 0, "hello\n"},
		{"stderr_captured", "echo err >&2", 0, "err\n"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, out, err := runOneValidationCommand(context.Background(), tt.cmd, "", nil)
			if err != nil {
				t.Errorf("runOneValidationCommand(%q): unexpected err: %v", tt.cmd, err)
			}
			if code != tt.wantCode {
				t.Errorf("runOneValidationCommand(%q): code = %d, want %d", tt.cmd, code, tt.wantCode)
			}
			if tt.wantOut != "" && out != tt.wantOut {
				t.Errorf("runOneValidationCommand(%q): out = %q, want %q", tt.cmd, out, tt.wantOut)
			}
		})
	}
}

// TestRunOneValidationCommand_OutputTruncated verifies output > 64KB is
// trimmed to the last 64KB and the tail content is preserved.
func TestRunOneValidationCommand_OutputTruncated(t *testing.T) {
	t.Parallel()

	// Emit 66 KB of 'A' followed by a recognisable marker; total > validationTailSize.
	shellCmd := `python3 -c "import sys; sys.stdout.write('A'*67584); sys.stdout.write('ENDMARKER')"`

	code, out, err := runOneValidationCommand(context.Background(), shellCmd, "", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(out) > validationTailSize {
		t.Errorf("output length %d exceeds tail limit %d", len(out), validationTailSize)
	}
	if !strings.HasSuffix(out, "ENDMARKER") {
		suffix := out
		if len(suffix) > 50 {
			suffix = suffix[len(suffix)-50:]
		}
		t.Errorf("expected output to end with ENDMARKER, got ...%q", suffix)
	}
}

// TestRunOneValidationCommand_ContextTimeout verifies that a context deadline
// kills the command and returns a non-zero exit code plus a context error.
func TestRunOneValidationCommand_ContextTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	code, _, err := runOneValidationCommand(ctx, "sleep 10", "", nil)
	if err == nil {
		t.Error("expected context error, got nil")
	}
	if code == 0 {
		t.Error("expected non-zero exit code for killed process")
	}
}

// TestRunValidationCommands_NilCommands verifies nil slice is a no-op.
func TestRunValidationCommands_NilCommands(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{validationCommands: nil, env: []string{}}
	idx, code, tail, err := sp.runValidationCommands(context.Background(), proc)
	if idx != -1 || code != 0 || tail != "" || err != nil {
		t.Errorf("nil commands: got (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx, code, tail, err)
	}
}

// TestRunValidationCommands_EmptySlice verifies empty slice is a no-op.
func TestRunValidationCommands_EmptySlice(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{validationCommands: []string{}, env: []string{}}
	idx, code, tail, err := sp.runValidationCommands(context.Background(), proc)
	if idx != -1 || code != 0 || tail != "" || err != nil {
		t.Errorf("empty commands: got (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx, code, tail, err)
	}
}

// TestRunValidationCommands_AllPass verifies all-zero exits return (-1,0,"",nil).
func TestRunValidationCommands_AllPass(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{
		validationCommands: []string{"true", "true", "true"},
		env:                []string{},
	}
	idx, code, tail, err := sp.runValidationCommands(context.Background(), proc)
	if idx != -1 || code != 0 || tail != "" || err != nil {
		t.Errorf("all pass: got (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx, code, tail, err)
	}
}

// TestRunValidationCommands_FirstFailShortCircuits verifies execution stops at the
// first failure (the second command must not run).
func TestRunValidationCommands_FirstFailShortCircuits(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{
		validationCommands: []string{"false", "echo SHOULD_NOT_RUN"},
		env:                []string{},
	}
	idx, code, tail, err := sp.runValidationCommands(context.Background(), proc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != 0 {
		t.Errorf("failedIdx = %d, want 0", idx)
	}
	if code == 0 {
		t.Errorf("code = 0, want non-zero")
	}
	if strings.Contains(tail, "SHOULD_NOT_RUN") {
		t.Error("second command ran despite first failure")
	}
}

// TestRunValidationCommands_SecondFailCapturesExitAndOutput verifies that when the
// second command fails, the returned index, exit code, and output tail are correct.
func TestRunValidationCommands_SecondFailCapturesExitAndOutput(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{
		validationCommands: []string{"true", "echo FAIL_OUTPUT; exit 7"},
		env:                []string{},
	}
	idx, code, tail, err := sp.runValidationCommands(context.Background(), proc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != 1 {
		t.Errorf("failedIdx = %d, want 1", idx)
	}
	if code != 7 {
		t.Errorf("code = %d, want 7", code)
	}
	if !strings.Contains(tail, "FAIL_OUTPUT") {
		t.Errorf("tail %q does not contain FAIL_OUTPUT", tail)
	}
}

// TestRunValidationCommands_Timeout verifies that a per-command timeout kills
// the command and returns a failure (not a parent-ctx error).
// NOT parallel — mutates package-level validationCommandTimeout.
func TestRunValidationCommands_Timeout(t *testing.T) {
	orig := validationCommandTimeout
	validationCommandTimeout = 100 * time.Millisecond
	t.Cleanup(func() { validationCommandTimeout = orig })

	sp := New(Config{Clock: clock.Real()})
	proc := &processInfo{
		validationCommands: []string{"sleep 10"},
		env:                []string{},
	}
	idx, code, _, err := sp.runValidationCommands(context.Background(), proc)
	if err != nil {
		t.Errorf("unexpected parent ctx error: %v", err)
	}
	if idx != 0 {
		t.Errorf("failedIdx = %d, want 0", idx)
	}
	if code == 0 {
		t.Error("expected non-zero exit code for killed/timed-out command")
	}
}
