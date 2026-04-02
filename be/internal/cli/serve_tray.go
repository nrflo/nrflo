//go:build tray

package cli

import "be/internal/tray"

func runWithTray(port int, agentCount tray.AgentCountFn, onStart func(), onQuit func()) {
	tray.Run(port, agentCount, onStart, onQuit)
}

const trayAvailable = true
