// Package cli implements the agent statusLine command.
// Note: api-mode (apirun) Claude sessions never invoke statusLine — this path
// is only exercised by CLI-mode (claude --settings statusLine) agents.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type statusLinePayload struct {
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
}

var agentStatuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Render Claude statusLine and forward context usage to server (used by Claude --settings statusLine)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil
		}

		var payload statusLinePayload
		_ = json.Unmarshal(raw, &payload)

		model := payload.Model.DisplayName
		if model == "" {
			model = "n/a"
		}
		cwd := payload.Workspace.CurrentDir
		if cwd == "" {
			cwd = "n/a"
		}

		// Render status line to stdout before any potential early-returns.
		out := cmd.OutOrStdout()
		useColor := isatty.IsTerminal(os.Stdout.Fd())
		if payload.ContextWindow.UsedPercentage != nil {
			pct := *payload.ContextWindow.UsedPercentage
			pctStr := fmt.Sprintf("%.0f%%", pct)
			if useColor {
				var color string
				switch {
				case pct < 60:
					color = "\x1b[32m"
				case pct < 85:
					color = "\x1b[33m"
				default:
					color = "\x1b[31m"
				}
				fmt.Fprintf(out, "%s%s %s Ctx: %s\x1b[0m\n", color, model, cwd, pctStr)
			} else {
				fmt.Fprintf(out, "%s %s Ctx: %s\n", model, cwd, pctStr)
			}
		} else {
			if useColor {
				fmt.Fprintf(out, "\x1b[32m%s %s Ctx: ?\x1b[0m\n", model, cwd)
			} else {
				fmt.Fprintf(out, "%s %s Ctx: ?\n", model, cwd)
			}
		}

		// context_update dispatch: requires session_id, used_percentage, and server running.
		if payload.ContextWindow.UsedPercentage != nil && GetSessionID() != "" && GetClient().IsServerRunning() {
			pct := *payload.ContextWindow.UsedPercentage
			contextLeft := int(100 - pct)
			if contextLeft < 0 {
				contextLeft = 0
			}
			if contextLeft > 100 {
				contextLeft = 100
			}
			sid := GetSessionID()
			reqParams := map[string]interface{}{
				"session_id":   sid,
				"context_left": contextLeft,
			}
			type result struct{ err error }
			ch := make(chan result, 1)
			go func() {
				ch <- result{err: GetClient().ExecuteAndUnmarshal("agent.context_update", reqParams, nil)}
			}()
			select {
			case <-ch:
			case <-time.After(1 * time.Second):
			}
		}

		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentStatuslineCmd)
}
