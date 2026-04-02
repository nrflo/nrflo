//go:build tray

package cli

import "be/internal/tray"

func runWithTray(port int, onStart func(), onQuit func(), hasRunningAgents func() bool) {
	tray.Run(port, onStart, onQuit, hasRunningAgents)
}

const trayAvailable = true
