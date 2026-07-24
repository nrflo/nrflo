package spawner

// appServerArgs returns the `codex app-server` argv with native delegation
// blocked via codexAgentsArgs (`-c agents.enabled=false`), placed first so the
// security-critical override sits adjacent to the subcommand. Kept pure (no
// exec) so it is unit-testable without running the codex binary.
func appServerArgs() []string {
	args := []string{"app-server"}
	args = append(args, codexAgentsArgs()...)
	args = append(args, codexProjectDocArgs()...)
	args = append(args, codexAutoCompactArgs()...)
	return args
}
