# Local Providers (Ollama and other OpenAI-compatible servers)

nrflo can point API-mode agents at a self-hosted OpenAI-compatible server
(Ollama, LM Studio, llama.cpp) instead of a cloud provider. This page is the
setup recipe for running a local model on Apple Silicon and wiring it into
System Agents.

## Prerequisites

- `api_mode_enabled` turned on for the server (Settings → Admin).
- Ollama >= 0.19 installed and running (`ollama serve`).
- Apple Silicon Mac with 32GB unified memory (recommended baseline for the
  models below with headroom for the OS and other apps).

## IMPORTANT: use `api_wire=chat_completions`

Ollama's `/v1` endpoint only implements the OpenAI **Chat Completions** API,
not the newer stateful **Responses** API. The `custom_providers` registry
defaults `api_wire` to `responses` — if you accept that default against
Ollama, requests will fail. **You must explicitly set `api_wire` to
`chat_completions`** when registering an Ollama-backed provider.

## Recommended models (32GB Apple Silicon)

- **Qwen3.5-4B** (thinking mode off) — fast, low memory footprint, good
  tool-calling reliability for extractor/summarizer-style agents.
- **Gemma 4 E4B** — alternative general-purpose local model in the same size
  class.

Pull a model with Ollama before registering it in nrflo, e.g.:

```
ollama pull qwen3.5:4b
```

## Register the provider

In the web UI, go to Settings → Custom Providers and add a row:

| Field | Value |
|---|---|
| Name | `local-ollama` (or any unique slug) |
| Base URL | `http://localhost:11434/v1` |
| API Wire | `chat_completions` |
| API Key | leave blank — Ollama does not require one |
| Enabled | yes |

The connection-check button probes the base URL's `/models` endpoint before
you save, so you can confirm Ollama is reachable first.

## Add a model row

Custom-provider models are **API-only**: the model row's `provider` field is
the custom provider's `name`, and only `api_model` (plus `api_context`,
`api_efforts`, `default_effort`) needs to be set — leave `cli_model` empty,
since there is no CLI backend for a local server. Set `api_model` to the
Ollama model tag (e.g. `qwen3.5:4b`).

## Point System Agents at the local model

Under Settings → System Agents, any agent with `execution_mode=api` can be
repointed at the new model — the model dropdown is filtered to
`execution_mode`-compatible models, so only API-mode/api-only rows show up
for api-mode agents. Typical low-cost candidates to move to a local model
first: the delegate T2 extractor tier (`_t2_extractor`) and
`context-saver-api`, since both run frequently and are latency-tolerant.

## Latency expectations

A local 4B-class model on Apple Silicon is materially slower per-token than
a hosted cloud API and has no cross-request caching, so expect noticeably
longer turnaround on any agent pointed at it — acceptable for background
tasks like context saving or short extraction, but not a drop-in replacement
for interactive-latency-sensitive agents.

## Restrictions

- Custom providers only work when `api_mode_enabled` is on; they have no CLI
  equivalent.
- Model rows backed by a custom provider are always API-only (no
  `cli_model`).
- No credential ladder: the `api_key` on the `custom_providers` row is sent
  as-is (blank is valid, as with Ollama).
