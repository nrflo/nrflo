# claudecfg

Edits Claude Code's global config (`$CLAUDE_CONFIG_DIR/.claude.json`, else `$HOME/.claude.json`).

`TrustFolder` pre-accepts the CLI's folder-trust dialog, which appears even under `--dangerously-skip-permissions` and defaults to "No, exit" — the prompt's submitting Enter would kill the launch. Every Claude spawn path calls it for its work dir (spawner adapter, user session, console engine); `service/claude_trust.go` also trusts the worktree base and project roots at startup and project create/update, because Claude honors a trusted ancestor and concurrent claude processes can rewrite `.claude.json` and drop a fresh per-folder entry.

Writes are atomic and process-serialized; inside a test binary (`testing.Testing()`) it only writes a config under the temp dir, so tests can never touch a developer's real file.
