package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

//go:embed agent_manual.md
var agentManual string

// DistFS returns the embedded UI distribution filesystem.
// Returns fs.Sub rooted at "dist" so files are accessible without the prefix.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// AgentManual returns the embedded agent manual markdown content.
func AgentManual() string {
	return agentManual
}
