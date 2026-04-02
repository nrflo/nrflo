//go:build !tray

package cli

func runWithTray(port int, _ func() (int, error), onStart func(), _ func()) {
	// no-op: tray not available, call onStart directly
	onStart()
}

const trayAvailable = false
