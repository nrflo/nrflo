package api

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/orchestrator"
	ptyPkg "be/internal/pty"
	"be/internal/refinery"
	"be/internal/spawner"
	"be/internal/ws"
)

// wireRefineryFoldSpawner builds a minimal long-lived spawner (same shape as
// the observer spawner in NewServer) whose only job is running the one-off
// `_refinery-cli` headless child a cli_interactive chain-entry fold attempt
// spawns, and wires it into refineryMgr via SetCLIFolder
// (refinery.Manager.cliFolder / spawner.Spawner.RunRefineryFold). The aux
// hooks are mandatory: the fold child inherits them via childSessionHooks,
// and without the auxSpawners registration the socket bridge serves it an
// empty tools/list and drops its heartbeats, so it can never write its
// digest and dies at the fold deadline.
func wireRefineryFoldSpawner(refineryMgr *refinery.Manager, orch *orchestrator.Orchestrator, pool *db.Pool, clk clock.Clock, hub *ws.Hub, dataPath string, ptyMgr *ptyPkg.Manager) {
	refineryFoldSpawner := spawner.New(spawner.Config{
		Pool:                pool,
		Clock:               clk,
		WSHub:               hub,
		DataPath:            dataPath,
		PTYManager:          ptyMgr,
		OnSessionRegister:   orch.RegisterAuxSpawner,
		OnSessionUnregister: orch.UnregisterAuxSpawner,
	})
	refineryMgr.SetCLIFolder(refineryFoldSpawner)
}
