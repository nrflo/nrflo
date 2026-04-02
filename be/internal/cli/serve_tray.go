//go:build tray

package cli

import "be/internal/tray"

func runWithTray(port int, onStart func(), onQuit func()) {
	tray.Run(port, onStart, onQuit)
}

const trayAvailable = true
