package spawner

import (
	"errors"
	"testing"
)

// TestCallbackError_ErrorMessage tests that CallbackError implements error interface
// and returns the expected error message format.
func TestCallbackError_ErrorMessage(t *testing.T) {
	err := &CallbackError{
		Level:        1,
		Instructions: "Fix the bug in layer 0",
		AgentType:    "qa-verifier",
	}

	expected := "callback to layer 1"
	if got := err.Error(); got != expected {
		t.Errorf("expected error message '%s', got '%s'", expected, got)
	}
}

// TestCallbackError_ErrorsAs tests that errors.As correctly detects CallbackError
func TestCallbackError_ErrorsAs(t *testing.T) {
	err := &CallbackError{
		Level:        2,
		Instructions: "Re-run from layer 2",
		AgentType:    "implementor",
	}

	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatal("errors.As failed to detect CallbackError")
	}

	if cbErr.Level != 2 {
		t.Errorf("expected Level=2, got %d", cbErr.Level)
	}
	if cbErr.Instructions != "Re-run from layer 2" {
		t.Errorf("expected Instructions='Re-run from layer 2', got '%s'", cbErr.Instructions)
	}
	if cbErr.AgentType != "implementor" {
		t.Errorf("expected AgentType='implementor', got '%s'", cbErr.AgentType)
	}
}

// TestCallbackError_ErrorsAsNonCallback tests that errors.As returns false for non-callback errors
func TestCallbackError_ErrorsAsNonCallback(t *testing.T) {
	err := errors.New("regular error")

	var cbErr *CallbackError
	if errors.As(err, &cbErr) {
		t.Error("errors.As should not detect CallbackError for regular errors")
	}
}

// TestCallbackError_ZeroLevel tests callback with level 0 (valid)
func TestCallbackError_ZeroLevel(t *testing.T) {
	err := &CallbackError{
		Level:        0,
		Instructions: "Start from beginning",
		AgentType:    "verifier",
	}

	expected := "callback to layer 0"
	if got := err.Error(); got != expected {
		t.Errorf("expected error message '%s', got '%s'", expected, got)
	}

	if err.Level != 0 {
		t.Errorf("expected Level=0, got %d", err.Level)
	}
}

// TestCallbackError_MultipleErrorWrapping tests that CallbackError can be wrapped and unwrapped
func TestCallbackError_MultipleErrorWrapping(t *testing.T) {
	cbErr := &CallbackError{
		Level:        3,
		Instructions: "Callback instructions",
		AgentType:    "analyzer",
	}

	// Wrap the error
	wrapped := errors.Join(cbErr, errors.New("additional context"))

	// Verify errors.As still works
	var unwrapped *CallbackError
	if !errors.As(wrapped, &unwrapped) {
		t.Fatal("errors.As failed to unwrap CallbackError")
	}

	if unwrapped.Level != 3 {
		t.Errorf("expected Level=3, got %d", unwrapped.Level)
	}
	if unwrapped.Instructions != "Callback instructions" {
		t.Errorf("expected Instructions='Callback instructions', got '%s'", unwrapped.Instructions)
	}
}

// TestCallbackError_DifferentLevels tests CallbackError with various level values
func TestCallbackError_DifferentLevels(t *testing.T) {
	testCases := []struct {
		level    int
		expected string
	}{
		{0, "callback to layer 0"},
		{1, "callback to layer 1"},
		{5, "callback to layer 5"},
		{10, "callback to layer 10"},
		{99, "callback to layer 99"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			err := &CallbackError{
				Level:        tc.level,
				Instructions: "test",
				AgentType:    "test-agent",
			}

			if got := err.Error(); got != tc.expected {
				t.Errorf("expected error message '%s', got '%s'", tc.expected, got)
			}
		})
	}
}
