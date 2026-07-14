# Console Reference

Flow mechanics for the `console` package. Not auto-loaded; read before changing the flows below. Invariants + pointers live in [CLAUDE.md](CLAUDE.md).

## Console driver model resolution

`--model` resolves against the `cli_models` registry (`resolveCLIModel`, `cli/console_client.go`): a matching enabled row supplies `mapped_model` **plus `reasoning_effort`/`fallback_models`** — registry ids are many-to-one on `mapped_model` (`codex_gpt55_high`/`_normal` both map to `gpt-5.5`), so dropping effort would silently launch a weaker model than the user named. A row belonging to the other `cli_type`, or a disabled one, errors before launch; an id absent from the registry falls back to the driver's own `adapter.MapModel`/`GetReasoningEffort`. Claude takes effort as `--effort`/`--fallback-model` (as the managed path does); the codex TUI has no effort flag, so it takes `-c model_reasoning_effort="<v>"` (it cannot be appended to the profile `config.toml`, which ends in a `[projects."<dir>"]` table).
