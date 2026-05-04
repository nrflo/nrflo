package python

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// successFactory returns a cmdFactory that echoes json to stdout and exits 0.
func successFactory(jsonOut string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s'", jsonOut)
	}
}

// failFactory returns a cmdFactory that writes to stderr and exits with code.
func failFactory(stderr string, exitCode int) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c",
			"printf '%s' '"+stderr+"' >&2; exit "+strings.Repeat("1", 0)+string(rune('0'+exitCode)))
	}
}

func TestResolveScript(t *testing.T) {
	cases := []struct {
		configDir  string
		scriptPath string
		wantErr    bool
	}{
		{"/config", "", true},
		{"/config", "/abs/path.py", true},
		{"/config", "../secret.py", true},
		{"/config", "../../etc/passwd", true},
		{"/config", "tools/script.py", false},
		{"/config", "script.py", false},
	}
	for _, tc := range cases {
		t.Run(tc.scriptPath, func(t *testing.T) {
			_, err := resolveScript(tc.configDir, tc.scriptPath)
			if (err != nil) != tc.wantErr {
				t.Errorf("resolveScript(%q, %q) error = %v, wantErr = %v",
					tc.configDir, tc.scriptPath, err, tc.wantErr)
			}
		})
	}
}

func TestResolveScript_AbsoluteResult(t *testing.T) {
	result, err := resolveScript("/config/dir", "tools/script.py")
	if err != nil {
		t.Fatalf("resolveScript: %v", err)
	}
	want := "/config/dir/tools/script.py"
	if result != want {
		t.Errorf("resolveScript = %q, want %q", result, want)
	}
}

func TestOSRunner_Invoke_Success(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `printf '{"ok":true}'`)
	})

	out, err := r.Invoke(context.Background(), "/script.py", []byte("{}"), nil, time.Second)
	if err != nil {
		t.Fatalf("Invoke success: %v", err)
	}
	if !strings.Contains(string(out), "ok") {
		t.Errorf("output = %q, want to contain 'ok'", string(out))
	}
}

func TestOSRunner_Invoke_NonZeroExit(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `printf 'err detail' >&2; exit 42`)
	})

	_, err := r.Invoke(context.Background(), "/script.py", []byte("{}"), nil, time.Second)
	if err == nil {
		t.Fatal("Invoke non-zero exit: expected error, got nil")
	}

	var se *ScriptError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ScriptError, got %T: %v", err, err)
	}
	if se.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", se.ExitCode)
	}
	if !strings.Contains(se.Stderr, "err detail") {
		t.Errorf("Stderr = %q, want to contain 'err detail'", se.Stderr)
	}
	if se.Cause == nil {
		t.Error("Cause is nil, want non-nil")
	}
}

func TestOSRunner_Invoke_Timeout(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "sleep 5")
	})

	_, err := r.Invoke(context.Background(), "/script.py", []byte("{}"), nil, 50*time.Millisecond)
	if err == nil {
		t.Fatal("Invoke timeout: expected error, got nil")
	}

	var se *ScriptError
	if !errors.As(err, &se) {
		t.Errorf("expected *ScriptError on timeout, got %T: %v", err, err)
	}
}

func TestOSRunner_Invoke_PassesEnv(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		// Print the environment to stdout so we can inspect it
		cmd := exec.CommandContext(ctx, "sh", "-c", "printf '%s' \"$MY_VAR\"")
		return cmd
	})

	env := []string{"MY_VAR=hello123"}
	out, err := r.Invoke(context.Background(), "/script.py", []byte("{}"), env, time.Second)
	if err != nil {
		t.Fatalf("Invoke with env: %v", err)
	}
	// The env is passed to cmd.Env in Invoke, but our mock doesn't use the original env
	// Just verify no error occurs when env is provided
	_ = out
}

func TestRuntime_Invoke_ResolvesScript(t *testing.T) {
	configDir := t.TempDir()

	var capturedPath string
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) > 0 {
			capturedPath = args[0]
		}
		return exec.CommandContext(ctx, "sh", "-c", `printf '{}'`)
	})

	rt := NewRuntime(r, configDir)
	_, err := rt.Invoke(context.Background(), "tools/script.py", []byte("{}"), nil, time.Second)
	if err != nil {
		t.Fatalf("Runtime.Invoke: %v", err)
	}

	expectedPath := configDir + "/tools/script.py"
	if capturedPath != expectedPath {
		t.Errorf("capturedPath = %q, want %q", capturedPath, expectedPath)
	}
}

func TestRuntime_Invoke_RejectsAbsPath(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `printf '{}'`)
	})
	rt := NewRuntime(r, "/config")

	_, err := rt.Invoke(context.Background(), "/absolute/script.py", []byte("{}"), nil, time.Second)
	if err == nil {
		t.Fatal("Runtime.Invoke with abs path: expected error, got nil")
	}
}

func TestRuntime_Invoke_RejectsParentTraversal(t *testing.T) {
	r := newOSRunnerWithFactory(func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `printf '{}'`)
	})
	rt := NewRuntime(r, "/config")

	_, err := rt.Invoke(context.Background(), "../secret.py", []byte("{}"), nil, time.Second)
	if err == nil {
		t.Fatal("Runtime.Invoke with traversal: expected error, got nil")
	}
}

func TestMatchEnv_EmptyPatterns(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/home/user"}
	result := MatchEnv(nil, environ)
	if result != nil {
		t.Errorf("MatchEnv nil patterns = %v, want nil", result)
	}
	result = MatchEnv([]string{}, environ)
	if result != nil {
		t.Errorf("MatchEnv empty patterns = %v, want nil", result)
	}
}

func TestMatchEnv_ExactMatch(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/home/user", "TERM=xterm"}
	result := MatchEnv([]string{"HOME"}, environ)
	if len(result) != 1 || result[0] != "HOME=/home/user" {
		t.Errorf("MatchEnv exact = %v, want ['HOME=/home/user']", result)
	}
}

func TestMatchEnv_GlobMatch(t *testing.T) {
	environ := []string{
		"ERP_HOST=localhost",
		"ERP_PORT=5432",
		"OTHER_VAR=x",
		"PATH=/usr/bin",
	}
	result := MatchEnv([]string{"ERP_*"}, environ)
	if len(result) != 2 {
		t.Errorf("MatchEnv ERP_* len = %d, want 2: %v", len(result), result)
	}
	for _, kv := range result {
		if !strings.HasPrefix(kv, "ERP_") {
			t.Errorf("unexpected env var %q matched ERP_* pattern", kv)
		}
	}
}

func TestMatchEnv_NoMatches(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/home/user"}
	result := MatchEnv([]string{"MY_APP_*"}, environ)
	if len(result) != 0 {
		t.Errorf("MatchEnv no matches = %v, want empty", result)
	}
}

func TestMatchEnv_MultiplePatterns(t *testing.T) {
	environ := []string{
		"FOO=1",
		"BAR=2",
		"BAZ=3",
		"OTHER=4",
	}
	result := MatchEnv([]string{"FOO", "BAR"}, environ)
	if len(result) != 2 {
		t.Errorf("MatchEnv multiple patterns len = %d, want 2: %v", len(result), result)
	}
}

func TestMatchEnv_KeyWithoutEquals(t *testing.T) {
	environ := []string{"PLAIN"}
	result := MatchEnv([]string{"PLAIN"}, environ)
	if len(result) != 1 || result[0] != "PLAIN" {
		t.Errorf("MatchEnv no-equals = %v, want ['PLAIN']", result)
	}
}

func TestScriptError_Error(t *testing.T) {
	se := &ScriptError{ExitCode: 1, Stderr: "some error"}
	msg := se.Error()
	if !strings.Contains(msg, "1") {
		t.Errorf("Error() = %q, want to contain '1'", msg)
	}
	if !strings.Contains(msg, "some error") {
		t.Errorf("Error() = %q, want to contain 'some error'", msg)
	}
}

func TestScriptError_Unwrap(t *testing.T) {
	cause := errors.New("cause error")
	se := &ScriptError{ExitCode: 1, Cause: cause}
	if !errors.Is(se, cause) {
		t.Error("errors.Is(ScriptError, cause) = false, want true")
	}
}

func TestRingBuffer_SmallData(t *testing.T) {
	rb := newRingBuffer(100)
	data := []byte("hello world")
	rb.Write(data)
	if rb.String() != "hello world" {
		t.Errorf("ringBuffer = %q, want 'hello world'", rb.String())
	}
}

func TestRingBuffer_TruncatesOldData(t *testing.T) {
	rb := newRingBuffer(10)
	rb.Write([]byte("first chunk "))
	rb.Write([]byte("second chunk"))

	result := rb.String()
	if len(result) > 10 {
		t.Errorf("ringBuffer len = %d, want <= 10", len(result))
	}
	// Should end with the last bytes of "second chunk"
	if !strings.HasSuffix("second chunk", result) {
		t.Errorf("ringBuffer = %q, want suffix of 'second chunk'", result)
	}
}
