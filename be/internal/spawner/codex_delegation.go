package spawner

// codexAgentsArgs returns the `-c agents.enabled=false` pair for
// appServerArgs(), blocking native multi-agent delegation (spawn_agent,
// followup_task, send_message, wait_agent, interrupt_agent, list_agents) so a
// managed session can't spawn children invisible to nrflo.
//
// Evidence (probed live against codex-cli 0.145.0, 2026-07-23): the previous
// mechanism, repeatable `--disable multi_agent --disable multi_agent_v2
// --disable enable_fanout` flags, is a proven no-op — `codex exec` with those
// exact flags still lists all six delegation tools and `codex debug
// prompt-input` is byte-identical with and without them (enable_fanout is
// even stage=removed already). `agents` is the unified 0.145.0 config root
// (`AgentsToml{enabled, max_concurrent_threads_per_session, max_depth,
// default_subagent_model, default_subagent_reasoning_effort,
// job_max_runtime_seconds, interrupt_message}` plus per-role `agents.<name>`),
// accepted under --strict-config. `-c agents.enabled=false` drops all six
// delegation tools with no collateral damage to exec/apply_patch/update_plan/
// MCP tools, and deep-merges over a user's own `[agents]` table in
// config.toml (their other fields, e.g. max_depth, survive; only enabled is
// forced off) rather than clobbering it.
//
// Drift alarm: TestNativeOrchestrationCLI (-tags clitools) fails loudly if
// `agents.enabled` is renamed or the delegation tool surface changes.
func codexAgentsArgs() []string {
	return []string{"-c", "agents.enabled=false"}
}
