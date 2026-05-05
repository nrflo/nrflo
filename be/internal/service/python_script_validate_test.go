package service

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
)

// newFakeValidator constructs a PythonScriptValidator with injected lookPath and cmdFactory,
// avoiding any real python3 dependency in tests.
func newFakeValidator(lookPathFn func(string) (string, error), factory func(context.Context, string, ...string) *exec.Cmd) *PythonScriptValidator {
	return &PythonScriptValidator{
		lookPath:   lookPathFn,
		cmdFactory: factory,
	}
}

// echoFactory returns a cmdFactory that ignores its inputs and runs echo <output> instead.
func echoFactory(output string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", output)
	}
}

// fakePython3 returns a lookPath that always resolves python3 to a fake path.
func fakePython3() func(string) (string, error) {
	return func(file string) (string, error) {
		return "/fake/python3", nil
	}
}

// missingPython3 returns a lookPath that always fails to find python3.
func missingPython3() func(string) (string, error) {
	return func(file string) (string, error) {
		return "", fmt.Errorf("python3 not found in PATH")
	}
}

func TestPythonScriptValidator_ValidCode(t *testing.T) {
	v := newFakeValidator(
		fakePython3(),
		echoFactory(`{"ok":true}`),
	)

	result := v.Validate(context.Background(), `print("hello")`)
	if !result.OK {
		t.Errorf("Validate(valid code) OK = false, want true; error = %q", result.Error)
	}
	if result.Error != "" {
		t.Errorf("Validate(valid code) Error = %q, want empty", result.Error)
	}
	if result.Line != nil {
		t.Errorf("Validate(valid code) Line = %v, want nil", result.Line)
	}
	if result.Col != nil {
		t.Errorf("Validate(valid code) Col = %v, want nil", result.Col)
	}
}

func TestPythonScriptValidator_SyntaxError(t *testing.T) {
	line := 1
	col := 5
	v := newFakeValidator(
		fakePython3(),
		echoFactory(`{"ok":false,"error":"invalid syntax","line":1,"col":5}`),
	)

	result := v.Validate(context.Background(), `def f(:`)
	if result.OK {
		t.Error("Validate(syntax error) OK = true, want false")
	}
	if result.Error == "" {
		t.Error("Validate(syntax error) Error is empty, want non-empty")
	}
	if result.Line == nil {
		t.Error("Validate(syntax error) Line is nil, want non-nil")
	} else if *result.Line != line {
		t.Errorf("Validate(syntax error) Line = %d, want %d", *result.Line, line)
	}
	if result.Col == nil {
		t.Error("Validate(syntax error) Col is nil, want non-nil")
	} else if *result.Col != col {
		t.Errorf("Validate(syntax error) Col = %d, want %d", *result.Col, col)
	}
}

func TestPythonScriptValidator_MissingPython3GraceFullyDegrades(t *testing.T) {
	v := newFakeValidator(
		missingPython3(),
		nil, // cmdFactory should never be called
	)

	result := v.Validate(context.Background(), `print("hello")`)
	if !result.OK {
		t.Errorf("Validate(no python3) OK = false, want true (graceful degrade)")
	}
	if result.Error != "" {
		t.Errorf("Validate(no python3) Error = %q, want empty", result.Error)
	}
}

func TestPythonScriptValidator_InvalidJSONFromPython3(t *testing.T) {
	// When python3 outputs non-JSON, validator gracefully returns OK=true.
	v := newFakeValidator(
		fakePython3(),
		echoFactory(`not valid json`),
	)

	result := v.Validate(context.Background(), `x = 1`)
	if !result.OK {
		t.Errorf("Validate(invalid json output) OK = false, want true (graceful degrade)")
	}
}

func TestPythonScriptValidator_MultipleErrorFields(t *testing.T) {
	// Ensure all ValidationResult fields map correctly.
	line := 3
	col := 10
	v := newFakeValidator(
		fakePython3(),
		echoFactory(`{"ok":false,"error":"unexpected EOF","line":3,"col":10}`),
	)

	result := v.Validate(context.Background(), "x = (\n  1 +\n")
	if result.OK {
		t.Error("Validate() OK = true, want false")
	}
	if result.Error != "unexpected EOF" {
		t.Errorf("Error = %q, want %q", result.Error, "unexpected EOF")
	}
	if result.Line == nil || *result.Line != line {
		t.Errorf("Line = %v, want %d", result.Line, line)
	}
	if result.Col == nil || *result.Col != col {
		t.Errorf("Col = %v, want %d", result.Col, col)
	}
}

func TestPythonScriptValidator_EmptyCode(t *testing.T) {
	// Empty code is syntactically valid Python — python3 returns ok:true.
	v := newFakeValidator(
		fakePython3(),
		echoFactory(`{"ok":true}`),
	)

	result := v.Validate(context.Background(), "")
	if !result.OK {
		t.Errorf("Validate(empty code) OK = false, want true")
	}
}

func TestPythonScriptValidator_CancelledContext(t *testing.T) {
	// When context is cancelled before validate runs, validator should not panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	v := newFakeValidator(
		fakePython3(),
		func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo", `{"ok":true}`)
		},
	)

	// Should not panic regardless of result (cancelled ctx may cause cmd error).
	_ = v.Validate(ctx, `print("hello")`)
}
