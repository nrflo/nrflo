package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// runStatusline executes agentStatuslineCmd with the given stdin JSON and returns stdout content.
func runStatusline(t *testing.T, stdinJSON string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	agentStatuslineCmd.SetOut(&buf)
	agentStatuslineCmd.SetErr(&buf)
	agentStatuslineCmd.SetIn(strings.NewReader(stdinJSON))
	err := agentStatuslineCmd.RunE(agentStatuslineCmd, []string{})
	return buf.String(), err
}

// TestAgentStatuslineCmdStructure verifies command metadata and arg constraints.
func TestAgentStatuslineCmdStructure(t *testing.T) {
	if agentStatuslineCmd.Use != "statusline" {
		t.Errorf("statusline Use = %q, want 'statusline'", agentStatuslineCmd.Use)
	}

	if agentStatuslineCmd.Short == "" {
		t.Error("statusline Short should not be empty")
	}

	if agentStatuslineCmd.RunE == nil {
		t.Error("statusline missing RunE function")
	}

	if err := agentStatuslineCmd.Args(agentStatuslineCmd, []string{}); err != nil {
		t.Errorf("statusline should accept 0 args: %v", err)
	}

	if err := agentStatuslineCmd.Args(agentStatuslineCmd, []string{"unexpected"}); err == nil {
		t.Error("statusline should reject any positional args")
	}
}

// TestAgentStatuslineHappyPath verifies output contains pct, model, and cwd.
func TestAgentStatuslineHappyPath(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "test-session-abc")

	const payload = `{"context_window":{"used_percentage":42},"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/tmp/x"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("statusline happy path returned error: %v", err)
	}
	if !strings.Contains(out, "42%") {
		t.Errorf("output missing pct: got %q", out)
	}
	if !strings.Contains(out, "Sonnet") {
		t.Errorf("output missing model: got %q", out)
	}
	if !strings.Contains(out, "/tmp/x") {
		t.Errorf("output missing cwd: got %q", out)
	}
}

// TestAgentStatuslineMissingContextWindow verifies "?" is shown when used_percentage is absent.
func TestAgentStatuslineMissingContextWindow(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "test-session-abc")

	const payload = `{"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/tmp/x"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("missing context_window returned error: %v", err)
	}
	if !strings.Contains(out, "?") {
		t.Errorf("output missing '?' fallback when context_window absent: got %q", out)
	}
	if !strings.Contains(out, "Sonnet") {
		t.Errorf("output missing model name: got %q", out)
	}
}

// TestAgentStatuslineMissingSessionID verifies output is still rendered when NRF_SESSION_ID is unset.
func TestAgentStatuslineMissingSessionID(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"context_window":{"used_percentage":75},"model":{"display_name":"Opus"},"workspace":{"current_dir":"/home/user"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("missing session ID returned error: %v", err)
	}
	if !strings.Contains(out, "75%") {
		t.Errorf("output missing pct: got %q", out)
	}
	if !strings.Contains(out, "Opus") {
		t.Errorf("output missing model: got %q", out)
	}
}

// TestAgentStatuslineModelFallback verifies model defaults to "n/a" when absent.
func TestAgentStatuslineModelFallback(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"context_window":{"used_percentage":50},"workspace":{"current_dir":"/tmp"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("missing model returned error: %v", err)
	}
	if !strings.Contains(out, "n/a") {
		t.Errorf("output missing 'n/a' model fallback: got %q", out)
	}
}

// TestAgentStatuslineCwdFallback verifies cwd defaults to "n/a" when absent.
func TestAgentStatuslineCwdFallback(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"context_window":{"used_percentage":50},"model":{"display_name":"Haiku"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("missing cwd returned error: %v", err)
	}
	if !strings.Contains(out, "n/a") {
		t.Errorf("output missing 'n/a' cwd fallback: got %q", out)
	}
}

// TestAgentStatuslineNoColorInNonTTY verifies no ANSI escape codes when stdout is not a TTY.
func TestAgentStatuslineNoColorInNonTTY(t *testing.T) {
	// os.Stdout is not a TTY in test environments, so isatty.IsTerminal returns false.
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"context_window":{"used_percentage":90},"model":{"display_name":"Opus"},"workspace":{"current_dir":"/tmp"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("statusline returned error: %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output contains ANSI escape codes in non-TTY context: %q", out)
	}
}

// TestAgentStatuslineServerNotRunning verifies exit 0 when server is absent.
func TestAgentStatuslineServerNotRunning(t *testing.T) {
	// NRF_SESSION_ID is set so the code reaches IsServerRunning, but no server exists.
	t.Setenv("NRF_SESSION_ID", "test-session-xyz")

	const payload = `{"context_window":{"used_percentage":60},"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/home"}}`
	_, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("statusline with no running server should return nil, got: %v", err)
	}
}

// TestAgentStatuslineConsecutiveCalls verifies two back-to-back invocations both return nil.
func TestAgentStatuslineConsecutiveCalls(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "test-session-cons")

	const payload = `{"context_window":{"used_percentage":42},"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/tmp"}}`

	_, err1 := runStatusline(t, payload)
	_, err2 := runStatusline(t, payload)

	if err1 != nil {
		t.Errorf("first call returned error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second call returned error: %v", err2)
	}
}

// TestAgentStatuslineEmptyInput verifies graceful handling of empty stdin.
func TestAgentStatuslineEmptyInput(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	out, err := runStatusline(t, "")

	if err != nil {
		t.Errorf("empty stdin returned error: %v", err)
	}
	// With no JSON, all fields default to zero values → "n/a" fallbacks and "?" pct.
	if !strings.Contains(out, "n/a") {
		t.Errorf("empty stdin output should contain 'n/a' fallbacks: got %q", out)
	}
}

// TestAgentStatuslineInvalidJSON verifies graceful handling of malformed JSON.
func TestAgentStatuslineInvalidJSON(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	out, err := runStatusline(t, `{not valid json`)

	if err != nil {
		t.Errorf("invalid JSON returned error: %v", err)
	}
	if !strings.Contains(out, "n/a") {
		t.Errorf("invalid JSON output should contain 'n/a' fallbacks: got %q", out)
	}
}

// TestAgentStatuslineOutputFormat verifies the rendered line structure.
func TestAgentStatuslineOutputFormat(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"context_window":{"used_percentage":42},"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/tmp/x"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("statusline returned error: %v", err)
	}
	// Non-TTY format: "model cwd Ctx: pct%\n"
	expected := "Sonnet /tmp/x Ctx: 42%"
	if !strings.Contains(out, expected) {
		t.Errorf("output %q does not contain expected format %q", out, expected)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with newline: got %q", out)
	}
}

// TestAgentStatuslineFallbackFormat verifies "Ctx: ?" format when pct is absent.
func TestAgentStatuslineFallbackFormat(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	const payload = `{"model":{"display_name":"Haiku"},"workspace":{"current_dir":"/home"}}`
	out, err := runStatusline(t, payload)

	if err != nil {
		t.Errorf("statusline returned error: %v", err)
	}
	expected := "Haiku /home Ctx: ?"
	if !strings.Contains(out, expected) {
		t.Errorf("output %q does not contain expected format %q", out, expected)
	}
}

// TestAgentStatuslinePctBoundaries verifies formatted percentage at boundary values.
func TestAgentStatuslinePctBoundaries(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	cases := []struct {
		pct  float64
		want string
	}{
		{0, "0%"},
		{100, "100%"},
		{42, "42%"},
		{59, "59%"},
		{60, "60%"},
		{84, "84%"},
		{85, "85%"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("pct_%.0f", tc.pct), func(t *testing.T) {
			payload := fmt.Sprintf(
				`{"context_window":{"used_percentage":%.0f},"model":{"display_name":"M"},"workspace":{"current_dir":"/d"}}`,
				tc.pct,
			)
			out, err := runStatusline(t, payload)
			if err != nil {
				t.Errorf("pct=%.0f returned error: %v", tc.pct, err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("pct=%.0f: output %q missing %q", tc.pct, out, tc.want)
			}
		})
	}
}

// TestAgentStatuslineRegisteredUnderAgent verifies the command is reachable via agentCmd.
func TestAgentStatuslineRegisteredUnderAgent(t *testing.T) {
	subcmds := getCommandNames(agentCmd)
	if !contains(subcmds, "statusline") {
		t.Errorf("agentCmd missing 'statusline' subcommand. Got: %v", subcmds)
	}
}
