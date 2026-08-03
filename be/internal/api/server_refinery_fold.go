package api

import (
	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/refinery"
	"be/internal/spawner"
	"be/internal/ws"
)

// wireRefineryFoldSpawner builds a minimal long-lived spawner (same shape as
// the observer spawner in NewServer) whose only job is running the one-off
// `_refinery-cli` headless child a cli_interactive chain-entry fold attempt
// spawns, and wires it into refineryMgr via SetCLIFolder
// (refinery.Manager.cliFolder / spawner.Spawner.RunRefineryFold).
func wireRefineryFoldSpawner(refineryMgr *refinery.Manager, pool *db.Pool, clk clock.Clock, hub *ws.Hub, dataPath string, ptyMgr *ptyPkg.Manager) {
	refineryFoldSpawner := spawner.New(spawner.Config{
		Pool:       pool,
		Clock:      clk,
		WSHub:      hub,
		DataPath:   dataPath,
		PTYManager: ptyMgr,
	})
	refineryMgr.SetCLIFolder(refineryFoldSpawner)
}
