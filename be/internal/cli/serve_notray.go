//go:build !tray

package cli

func runWithTray(port int, onStart func(), onQuit func()) {
	// no-op: tray not available, call onStart directly
	onStart()
}

const trayAvailable = false
