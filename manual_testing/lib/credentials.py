"""Credential probes for api-mode manual-test runners.

`probe_oauth_token()` — Anthropic OAuth bearer token.
  Injects `ANTHROPIC_OAUTH_TOKEN` into the server env; resolved by
  `ResolveAPIKey` in
  `be/internal/spawner/apirun/provider/anthropic/credentials.go`.

`probe_openai_key()` — OpenAI API key.
  Injects `OPENAI_API_KEY` into the server env; resolved by `Resolve` in
  `be/internal/spawner/apirun/provider/openai/credentials.go`.

Both return `(value, reason)` on success or `(None, skip-reason)` when
no usable credential is reachable, so the runner can skip the whole
folder cleanly rather than hard-error."""

from __future__ import annotations

import json
import os
import subprocess
from typing import Any


KEYCHAIN_SERVICE = "Claude Code-credentials"


def probe_oauth_token() -> tuple[str | None, str]:
    """Return `(token, reason)`. `token` is `None` when no usable
    token is reachable; `reason` is then a one-line SKIP message.

    Resolution order:
      1. `ANTHROPIC_OAUTH_TOKEN` env var, if already set.
      2. `ANTHROPIC_API_KEY` env var, if it looks like an OAuth token.
      3. macOS Keychain generic password under service
         `Claude Code-credentials` — JSON payload, pull
         `claudeAiOauth.accessToken`.
    """
    tok = os.environ.get("ANTHROPIC_OAUTH_TOKEN", "").strip()
    if tok:
        return tok, "ok (env ANTHROPIC_OAUTH_TOKEN)"

    tok = os.environ.get("ANTHROPIC_API_KEY", "").strip()
    if tok.startswith("sk-ant-oat01-"):
        return tok, "ok (env ANTHROPIC_API_KEY)"

    if os.uname().sysname != "Darwin":
        return None, "no env token and not on macOS (Keychain unavailable)"

    try:
        proc = subprocess.run(
            ["security", "find-generic-password", "-s", KEYCHAIN_SERVICE, "-w"],
            capture_output=True, text=True, timeout=5,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired) as e:
        return None, f"`security` invocation failed: {e!r}"
    if proc.returncode != 0:
        return None, (
            f"Keychain entry {KEYCHAIN_SERVICE!r} not found "
            f"(security rc={proc.returncode})"
        )

    raw = proc.stdout.strip()
    if not raw:
        return None, f"Keychain entry {KEYCHAIN_SERVICE!r} empty"
    try:
        payload: Any = json.loads(raw)
    except json.JSONDecodeError as e:
        return None, f"Keychain payload is not JSON: {e}"

    tok = (
        payload.get("claudeAiOauth", {}).get("accessToken")
        if isinstance(payload, dict) else None
    )
    if not tok:
        return None, "Keychain JSON missing claudeAiOauth.accessToken"
    if not tok.startswith("sk-ant-oat01-"):
        return None, "Keychain token does not look like an OAuth token"
    return tok, "ok (keychain)"


def probe_openai_key() -> tuple[str | None, str]:
    """Return `(key, reason)`. `key` is `None` when no usable key is
    reachable; `reason` is then a one-line SKIP message.

    Resolution order (env only — no macOS Keychain):
      1. `OPENAI_API_KEY` env var.
      2. `CODEX_API_KEY` env var.
    """
    key = os.environ.get("OPENAI_API_KEY", "").strip()
    if key:
        return key, "ok (env OPENAI_API_KEY)"
    key = os.environ.get("CODEX_API_KEY", "").strip()
    if key:
        return key, "ok (env CODEX_API_KEY)"
    return None, "neither OPENAI_API_KEY nor CODEX_API_KEY is set"
