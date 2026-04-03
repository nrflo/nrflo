//go:build tray

package cli

import "be/internal/tray"

func runWithTray(host string, port int, agentCount tray.AgentCountFn, onStart func(), onQuit func()) {
	tray.Run(host, port, agentCount, onStart, onQuit)
}

const trayAvailable = true
