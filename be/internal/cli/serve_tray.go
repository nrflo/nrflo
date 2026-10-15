//go:build tray

package cli

import "be/internal/tray"

func runWithTray(host string, port int, agentCount tray.CountFn, unreadCount tray.CountFn, onStart func(), onQuit func()) {
	tray.Run(host, port, agentCount, unreadCount, onStart, onQuit)
}

const trayAvailable = true
