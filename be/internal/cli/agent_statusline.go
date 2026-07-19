// Package cli implements the agent statusLine command.
// Exercised by CLI-mode agents and api-via-cli hybrid sessions
// (both use claude --settings statusLine).
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

type rlWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       string   `json:"resets_at"`
}

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
	RateLimits *struct {
		FiveHour *rlWindow `json:"five_hour"`
		SevenDay *rlWindow `json:"seven_day"`
	} `json:"rate_limits"`
	Cost *struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
}

// rateLimitsUpdateParams is the agent.rate_limits_update payload sent to the server.
type rateLimitsUpdateParams struct {
	SessionID string    `json:"session_id"`
	FiveHour  *rlWindow `json:"five_hour,omitempty"`
	SevenDay  *rlWindow `json:"seven_day,omitempty"`
}

// buildRateLimitsParams maps the statusline rate_limits payload to the
// agent.rate_limits_update params, or (nil, false) when there is nothing to forward
// (no rate_limits present, or no session id to attribute them to).
func buildRateLimitsParams(p statusLinePayload, sessionID string) (*rateLimitsUpdateParams, bool) {
	if p.RateLimits == nil || sessionID == "" {
		return nil, false
	}
	if p.RateLimits.FiveHour == nil && p.RateLimits.SevenDay == nil {
		return nil, false
	}
	return &rateLimitsUpdateParams{
		SessionID: sessionID,
		FiveHour:  p.RateLimits.FiveHour,
		SevenDay:  p.RateLimits.SevenDay,
	}, true
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
		costSuffix := ""
		if payload.Cost != nil && payload.Cost.TotalCostUSD > 0 {
			costSuffix = fmt.Sprintf(" ~$%.2f", payload.Cost.TotalCostUSD)
		}
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
				fmt.Fprintf(out, "%s%s %s Ctx: %s%s\x1b[0m\n", color, model, cwd, pctStr, costSuffix)
			} else {
				fmt.Fprintf(out, "%s %s Ctx: %s%s\n", model, cwd, pctStr, costSuffix)
			}
		} else {
			if useColor {
				fmt.Fprintf(out, "\x1b[32m%s %s Ctx: ?%s\x1b[0m\n", model, cwd, costSuffix)
			} else {
				fmt.Fprintf(out, "%s %s Ctx: ?%s\n", model, cwd, costSuffix)
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

		// rate_limits dispatch: forward Claude subscription limits to the server so
		// it can surface them and drive precise rate-limit waits. Best-effort, async.
		if rlParams, ok := buildRateLimitsParams(payload, GetSessionID()); ok && GetClient().IsServerRunning() {
			type result struct{ err error }
			ch := make(chan result, 1)
			go func() {
				ch <- result{err: GetClient().ExecuteAndUnmarshal("agent.rate_limits_update", rlParams, nil)}
			}()
			select {
			case <-ch:
			case <-time.After(1 * time.Second):
			}
		}

		return nil
	},
}
