package tray

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"be/internal/logger"

	"fyne.io/systray"
)

// Run starts the systray on the main thread. onServerStart is called in a
// goroutine once the tray is ready. onQuit is called when the user quits
// (via menu or Ctrl+C). hasRunningAgents is checked before quit; if true,
// a native macOS confirmation dialog is shown. This function blocks until
// the tray exits.
func Run(port int, onServerStart func(), onQuit func(), hasRunningAgents func() bool) {
	systray.Run(func() {
		systray.SetTemplateIcon(iconBytes, iconBytes)
		systray.SetTooltip(fmt.Sprintf("nrflow server — port %d", port))

		mOpen := systray.AddMenuItem("Open nrflow", "Open web UI in browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop server and quit")

		// Listen for Ctrl+C / SIGTERM alongside the menu
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					_ = exec.Command("open", fmt.Sprintf("http://localhost:%d", port)).Start()
				case <-mQuit.ClickedCh:
					if hasRunningAgents() && !confirmQuit() {
						continue
					}
					onQuit()
					systray.Quit()
					return
				case <-sigCh:
					onQuit()
					systray.Quit()
					return
				}
			}
		}()

		go onServerStart()
	}, func() {
		// onExit — systray has shut down
	})
}

// confirmQuit shows a native macOS confirmation dialog via osascript.
// Returns true if the user clicked "Quit Anyway", false on Cancel or error.
func confirmQuit() bool {
	cmd := exec.Command("osascript", "-e",
		`display dialog "Agents are currently running. Quitting will terminate them." buttons {"Cancel", "Quit Anyway"} default button "Cancel" with icon caution`)
	if err := cmd.Run(); err != nil {
		logger.Info(context.Background(), "quit cancelled by user or osascript error", "error", err)
		return false
	}
	return true
}
