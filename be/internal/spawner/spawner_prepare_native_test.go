package spawner

import (
	"testing"

	"be/internal/model"
)

// TestNativeSpawnFields verifies per-adapter routing: native_tools reaches
// only claude, sandbox reaches only codex, and a nil def (global workflow
// defs invisible to loadAgentDefinition) yields no restriction.
func TestNativeSpawnFields(t *testing.T) {
	t.Parallel()

	def := &model.AgentDefinition{
		NativeTools: "Read,Grep",
		Sandbox:     model.SandboxReadOnly,
	}

	if nt, sb := nativeSpawnFields(def, "claude"); nt != "Read,Grep" || sb != "" {
		t.Errorf("claude: got (%q, %q), want (Read,Grep, \"\")", nt, sb)
	}
	if nt, sb := nativeSpawnFields(def, "codex"); nt != "" || sb != model.SandboxReadOnly {
		t.Errorf("codex: got (%q, %q), want (\"\", read-only)", nt, sb)
	}
	if nt, sb := nativeSpawnFields(def, "unknown"); nt != "" || sb != "" {
		t.Errorf("unknown adapter: got (%q, %q), want empty", nt, sb)
	}
	if nt, sb := nativeSpawnFields(nil, "claude"); nt != "" || sb != "" {
		t.Errorf("nil def: got (%q, %q), want empty", nt, sb)
	}
}
