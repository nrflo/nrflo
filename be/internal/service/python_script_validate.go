package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"be/internal/logger"
)

const pyValidateScript = `import ast, json, sys
try:
    ast.parse(open(sys.argv[1]).read(), filename=sys.argv[1])
    sys.stdout.write(json.dumps({"ok": True}) + "\n")
except SyntaxError as e:
    sys.stdout.write(json.dumps({"ok": False, "error": e.msg if hasattr(e, "msg") and e.msg else str(e), "line": e.lineno, "col": e.offset}) + "\n")
`

// ValidationResult holds the result of a Python syntax validation
type ValidationResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Line  *int   `json:"line,omitempty"`
	Col   *int   `json:"col,omitempty"`
}

// PythonScriptValidator validates Python code syntax using python3
type PythonScriptValidator struct {
	lookPath   func(file string) (string, error)
	cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewPythonScriptValidator creates a validator using the system python3
func NewPythonScriptValidator() *PythonScriptValidator {
	return &PythonScriptValidator{
		lookPath: exec.LookPath,
		cmdFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	}
}

// Validate checks Python code syntax. Gracefully degrades to OK=true if python3 is unavailable.
func (v *PythonScriptValidator) Validate(ctx context.Context, code string) ValidationResult {
	python3, err := v.lookPath("python3")
	if err != nil {
		logger.Warn(ctx, "python3 not found in PATH, skipping syntax validation")
		return ValidationResult{OK: true}
	}

	if err := os.MkdirAll("/tmp/nrflo", 0755); err != nil {
		return ValidationResult{OK: true}
	}

	tmpFile, err := os.CreateTemp("/tmp/nrflo", "validate-*.py")
	if err != nil {
		return ValidationResult{OK: true}
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		return ValidationResult{OK: true}
	}
	tmpFile.Close()

	valCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := v.cmdFactory(valCtx, python3, "-c", pyValidateScript, tmpPath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return ValidationResult{OK: false, Error: fmt.Sprintf("python3 error: %v", err)}
	}

	var raw struct {
		OK    bool    `json:"ok"`
		Error string  `json:"error"`
		Line  *int    `json:"line"`
		Col   *int    `json:"col"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &raw); err != nil {
		return ValidationResult{OK: true}
	}

	return ValidationResult{
		OK:    raw.OK,
		Error: raw.Error,
		Line:  raw.Line,
		Col:   raw.Col,
	}
}
