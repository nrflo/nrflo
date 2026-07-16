package model

// Codex sandbox modes accepted by the app-server thread/start `sandbox`
// param and the agent_definitions.sandbox column.
const (
	SandboxReadOnly         = "read-only"
	SandboxWorkspaceWrite   = "workspace-write"
	SandboxDangerFullAccess = "danger-full-access"
)

// NativeToolsNone is the agent_definitions.native_tools sentinel meaning
// "disable all native CLI tools" (claude --tools ""). An empty native_tools
// keeps the CLI's full built-in set.
const NativeToolsNone = "none"

// ValidSandbox reports whether s is an accepted sandbox value; empty is
// allowed and means the engine default (danger-full-access for spawns).
func ValidSandbox(s string) bool {
	switch s {
	case "", SandboxReadOnly, SandboxWorkspaceWrite, SandboxDangerFullAccess:
		return true
	}
	return false
}
