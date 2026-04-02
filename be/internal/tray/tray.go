package tray

import (
	"fmt"
	"os/exec"

	"fyne.io/systray"
)

// Run starts the systray on the main thread. onServerStart is called in a
// goroutine once the tray is ready. onQuit is called when the user clicks Quit.
// This function blocks until the tray exits.
func Run(port int, onServerStart func(), onQuit func()) {
	systray.Run(func() {
		systray.SetTemplateIcon(iconBytes, iconBytes)
		systray.SetTooltip(fmt.Sprintf("nrflow server — port %d", port))

		mOpen := systray.AddMenuItem("Open nrflow", "Open web UI in browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop server and quit")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					_ = exec.Command("open", fmt.Sprintf("http://localhost:%d", port)).Start()
				case <-mQuit.ClickedCh:
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
