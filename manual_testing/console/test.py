#!/usr/bin/env python3
"""Manual integration test for the console-chat surface.

LAUNCH MANUALLY ONLY. Spawns real claude/codex CLI engines (and the
in-process api engine) behind kind='console_chat' sessions. The folder
boots under the `claude` binary; C02 gates on codex itself and C03 on an
Anthropic OAuth token — the token is injected into the server env here,
best-effort, so the api engine can authenticate.

Usage:
    python3 manual_testing/console/test.py
    python3 manual_testing/console/test.py --only=C01 --parallel=1
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.credentials import probe_oauth_token, probe_openai_key  # noqa: E402
from lib.runner import run_all  # noqa: E402

from console import ALL_SCENARIOS  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=1)
    ap.add_argument("--model", default="haiku")
    ap.add_argument("--only", default=None)
    ap.add_argument("--timeout", type=float, default=300.0)
    ap.add_argument("--results", default=None,
                    help="optional JSON file path for per-scenario results")
    args = ap.parse_args()

    extra_env: dict[str, str] = {}
    tok, _reason = probe_oauth_token()
    if tok:
        extra_env["ANTHROPIC_OAUTH_TOKEN"] = tok
    key, _reason = probe_openai_key()
    if key:
        extra_env["OPENAI_API_KEY"] = key

    return run_all(
        scenarios=ALL_SCENARIOS,
        provider="console",
        model=args.model,
        binary="claude",
        mode="cli_interactive",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
        extra_env=extra_env,
    )


if __name__ == "__main__":
    sys.exit(main())
