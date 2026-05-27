package cli

// RegisterServerCommands adds server-side commands to the root command.
// The serve command runs by default when no subcommand is given.
func RegisterServerCommands() {
	rootCmd.Use = "nrflo_server"
	rootCmd.Short = "nrflo server"
	rootCmd.RunE = serveCmd.RunE
	rootCmd.Flags().AddFlagSet(serveCmd.Flags())
	rootCmd.AddCommand(serveCmd)
	// Agent infrastructure subcommands (MCP bridge + Claude hook/statusline
	// forwarders) the spawner invokes as short-lived `nrflo_server agent <cmd>`
	// processes that connect back to the running server over the Unix socket.
	rootCmd.AddCommand(agentInfraCmd)
}
