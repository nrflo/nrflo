package cli

import "github.com/spf13/cobra"

// agentInfraCmd is the server-hosted "agent" parent for infrastructure commands
// that are invoked by the spawner or Claude hooks — NOT by the agent itself:
// the MCP stdio bridge (serves mcp__nrflo__* tools) and the Claude hook event /
// statusline forwarders. Registered on nrflo_server by RegisterServerCommands.
// The agent-initiated commands (finished, fail, findings, …) are MCP tools, not
// CLI subcommands.
var agentInfraCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent infrastructure commands (MCP bridge + Claude hooks; invoked by the spawner)",
}

func init() {
	agentInfraCmd.AddCommand(agentMCPCmd)
	agentInfraCmd.AddCommand(agentMCPExternalCmd)
	agentInfraCmd.AddCommand(agentStatuslineCmd)

	agentRecordEventCmd.Flags().BoolVar(&agentRecordEventConsole, "console", false, "console (human-attended) session hook — blocking PreToolUse approval with a long deadline")
	agentInfraCmd.AddCommand(agentRecordEventCmd)

	agentContextUpdateCmd.Flags().Float64Var(&agentContextUpdatePctUsed, "pct-used", 0, "Percentage of context used")
	agentInfraCmd.AddCommand(agentContextUpdateCmd)
}
