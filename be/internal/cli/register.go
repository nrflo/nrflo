package cli

// RegisterServerCommands adds server-side commands to the root command.
func RegisterServerCommands() {
	rootCmd.Use = "nrworkflow_server"
	rootCmd.Short = "nrworkflow server"
	rootCmd.AddCommand(serveCmd)
}

// RegisterCLICommands adds client-facing commands to the root command.
func RegisterCLICommands() {
	rootCmd.Use = "nrworkflow"
	rootCmd.Short = "CLI tool for nrworkflow server"
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(findingsCmd)
	rootCmd.AddCommand(ticketsCmd)
	rootCmd.AddCommand(depsCmd)

	// Register ticketsCmd persistent flags (moved from tickets.go init).
	// Guard against double-registration which would cause a panic.
	if ticketsCmd.PersistentFlags().Lookup("server") == nil {
		ticketsCmd.PersistentFlags().StringVar(&ticketsServer, "server", "", "API server URL (default: NRWORKFLOW_API_URL or http://localhost:6587)")
		ticketsCmd.PersistentFlags().BoolVar(&ticketsJSON, "json", false, "Output as JSON")
	}
}
