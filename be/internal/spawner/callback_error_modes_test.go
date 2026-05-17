package spawner

import (
	"errors"
	"testing"
)

func TestCallbackError_ModeAgent(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "agent", TargetAgent: "qa-verifier"}
	if got := err.Error(); got != "callback agent=qa-verifier" {
		t.Errorf("got %q, want %q", got, "callback agent=qa-verifier")
	}
}

func TestCallbackError_ModeAgentEmptyTarget(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "agent", TargetAgent: ""}
	if got := err.Error(); got != "callback agent=" {
		t.Errorf("got %q, want %q", got, "callback agent=")
	}
}

func TestCallbackError_ModeChain(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "chain", Chain: []string{"phase1", "phase2"}}
	if got := err.Error(); got != "callback chain=[phase1,phase2]" {
		t.Errorf("got %q, want %q", got, "callback chain=[phase1,phase2]")
	}
}

func TestCallbackError_ModeChainEmpty(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "chain", Chain: []string{}}
	if got := err.Error(); got != "callback chain=[]" {
		t.Errorf("got %q, want %q", got, "callback chain=[]")
	}
}

func TestCallbackError_ModeChainNil(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "chain"}
	if got := err.Error(); got != "callback chain=[]" {
		t.Errorf("got %q, want %q", got, "callback chain=[]")
	}
}

func TestCallbackError_ModeEmptyDefaultsToLayer(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Level: 3}
	// zero-value Mode="" falls through to default case
	if got := err.Error(); got != "callback to layer 3" {
		t.Errorf("got %q, want %q", got, "callback to layer 3")
	}
}

func TestCallbackError_ModeLayerExplicit(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "layer", Level: 2}
	if got := err.Error(); got != "callback to layer 2" {
		t.Errorf("got %q, want %q", got, "callback to layer 2")
	}
}

func TestCallbackError_ModeLayerZeroLevel(t *testing.T) {
	t.Parallel()
	err := &CallbackError{Mode: "layer", Level: 0}
	if got := err.Error(); got != "callback to layer 0" {
		t.Errorf("got %q, want %q", got, "callback to layer 0")
	}
}

func TestCallbackError_ErrorsAsAllModes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  *CallbackError
	}{
		{"agent", &CallbackError{Mode: "agent", TargetAgent: "some-agent"}},
		{"chain", &CallbackError{Mode: "chain", Chain: []string{"p1", "p2"}}},
		{"layer", &CallbackError{Mode: "layer", Level: 1}},
		{"empty-mode", &CallbackError{Level: 2}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *CallbackError
			if !errors.As(tc.err, &got) {
				t.Fatalf("errors.As failed for mode=%q", tc.err.Mode)
			}
			if got.Mode != tc.err.Mode {
				t.Errorf("Mode: got %q, want %q", got.Mode, tc.err.Mode)
			}
		})
	}
}

func TestCallbackError_ErrorsAsWrapped(t *testing.T) {
	t.Parallel()

	wrapped := errors.Join(
		&CallbackError{Mode: "agent", TargetAgent: "implementor"},
		errors.New("extra"),
	)
	var got *CallbackError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As failed on wrapped CallbackError")
	}
	if got.TargetAgent != "implementor" {
		t.Errorf("TargetAgent: got %q, want %q", got.TargetAgent, "implementor")
	}
}
