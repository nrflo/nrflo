package cli

// RegisterServerCommands adds server-side commands to the root command.
func RegisterServerCommands() {
	rootCmd.Use = "nrflow_server"
	rootCmd.Short = "nrflow server"
	rootCmd.AddCommand(serveCmd)
}

// RegisterCLICommands adds client-facing commands to the root command.
func RegisterCLICommands() {
	rootCmd.Use = "nrflow"
	rootCmd.Short = "CLI tool for nrflow server"
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(findingsCmd)
	rootCmd.AddCommand(ticketsCmd)
	rootCmd.AddCommand(depsCmd)
	rootCmd.AddCommand(skipCmd)

	// Register ticketsCmd persistent flags (moved from tickets.go init).
	// Guard against double-registration which would cause a panic.
	if ticketsCmd.PersistentFlags().Lookup("server") == nil {
		ticketsCmd.PersistentFlags().StringVar(&ticketsServer, "server", "", "API server URL (default: NRFLOW_API_URL or http://localhost:6587)")
		ticketsCmd.PersistentFlags().BoolVar(&ticketsJSON, "json", false, "Output as JSON")
	}
}
