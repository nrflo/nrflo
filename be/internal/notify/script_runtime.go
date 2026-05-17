package notify

import (
	"be/internal/clock"
	"be/internal/repo"
	"be/internal/venv"
)

// ScriptRuntime holds shared dependencies for the script transport.
// Registered once from api/server.go via RegisterScriptRuntime.
type ScriptRuntime struct {
	ProjectRepo *repo.ProjectRepo
	VenvMgr     *venv.Manager
	EnvVarRepo  *repo.ProjectEnvVarRepo
	SessionRepo *repo.AgentSessionRepo
	Clock       clock.Clock
	SDKDir      string
	SocketPath  string
	NrfloHome   string
}

var scriptRuntime *ScriptRuntime

// RegisterScriptRuntime stores the runtime for use by scriptTransport.
// Called once from api/server.go NewServer.
func RegisterScriptRuntime(rt *ScriptRuntime) {
	scriptRuntime = rt
}

// getScriptRuntime returns the registered runtime, or nil if not set.
func getScriptRuntime() *ScriptRuntime {
	return scriptRuntime
}
