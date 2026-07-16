package spawner

import (
	"testing"

	"be/internal/model"
)

// TestEffectiveSpawnSandbox verifies the empty-value fallback to
// danger-full-access (today's autonomous default) and passthrough otherwise.
func TestEffectiveSpawnSandbox(t *testing.T) {
	t.Parallel()

	if got := effectiveSpawnSandbox(""); got != model.SandboxDangerFullAccess {
		t.Errorf("empty sandbox = %q, want %q", got, model.SandboxDangerFullAccess)
	}
	if got := effectiveSpawnSandbox(model.SandboxReadOnly); got != model.SandboxReadOnly {
		t.Errorf("read-only sandbox = %q, want passthrough", got)
	}
}

// TestThreadStartParams_SandboxPassthrough verifies the sandbox value lands
// verbatim in the thread/start params the app-server receives.
func TestThreadStartParams_SandboxPassthrough(t *testing.T) {
	t.Parallel()

	p := threadStartParams("gpt-5.2-codex", "/work", model.SandboxReadOnly, "never")
	if p["sandbox"] != model.SandboxReadOnly {
		t.Errorf("sandbox param = %v, want %q", p["sandbox"], model.SandboxReadOnly)
	}
	if p["approvalPolicy"] != "never" {
		t.Errorf("approvalPolicy param = %v, want never", p["approvalPolicy"])
	}
}
