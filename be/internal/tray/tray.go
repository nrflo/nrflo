package tray

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"be/internal/logger"

	"fyne.io/systray"
)

// AgentCountFn returns the number of running agents across all projects.
type AgentCountFn func() (int, error)

// Run starts the systray on the main thread. onServerStart is called in a
// goroutine once the tray is ready. onQuit is called when the user quits
// (via menu or Ctrl+C). agentCount is polled every 3s to update the icon
// with the running agent count. This function blocks until the tray exits.
func Run(port int, agentCount AgentCountFn, onServerStart func(), onQuit func()) {
	systray.Run(func() {
		initialIcon := renderIcon()
		systray.SetTemplateIcon(initialIcon, initialIcon)
		systray.SetTooltip(fmt.Sprintf("nrflow server — port %d", port))

		mOpen := systray.AddMenuItem("Open nrflow", "Open web UI in browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop server and quit")

		// Listen for Ctrl+C / SIGTERM alongside the menu
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		done := make(chan struct{})
		lastCount := 0

		// Poll agent count every 3s and update title on change
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					count, err := agentCount()
					if err != nil {
						continue
					}
					if count != lastCount {
						lastCount = count
						if count == 0 {
							systray.SetTitle("")
						} else {
							systray.SetTitle(fmt.Sprintf(" %d", count))
						}
					}
				case <-done:
					return
				}
			}
		}()

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					_ = exec.Command("open", fmt.Sprintf("http://localhost:%d", port)).Start()
				case <-mQuit.ClickedCh:
					count, _ := agentCount()
					if count > 0 && !confirmQuit() {
						continue
					}
					close(done)
					onQuit()
					systray.Quit()
					return
				case <-sigCh:
					close(done)
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
