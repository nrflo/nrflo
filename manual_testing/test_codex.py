#!/usr/bin/env python3
"""Manual integration test for the `codex` CLI provider.

LAUNCH MANUALLY ONLY. Not wired into make test / CI. Spawns a real
`codex` process against real OpenAI credentials.

Usage:
    python3 manual_testing/test_codex.py                  # mode=cli (default)
    python3 manual_testing/test_codex.py --mode=cli-interactive
"""

from __future__ import annotations

import argparse
import sys

from lib.runner import run_all


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", default="cli",
                    choices=["cli", "cli-interactive"])
    # codex/cli-interactive is slower under contention than the others
    # (PTY relay + reasoning-heavy model). Default to 3-wide; cli stays at 5.
    ap.add_argument("--parallel", type=int, default=None)
    ap.add_argument("--model", default="codex_gpt54_mini_low")
    ap.add_argument("--only", default=None,
                    help="comma-separated scenario substrings; e.g. s01,s07")
    # Bumped above the 180s default to give codex room under parallel load.
    ap.add_argument("--timeout", type=float, default=300.0,
                    help="per-scenario workflow-wait timeout in seconds")
    args = ap.parse_args()
    parallel = args.parallel
    if parallel is None:
        parallel = 3 if args.mode == "cli-interactive" else 5
    return run_all(
        provider="codex",
        model=args.model,
        binary="codex",
        mode=args.mode,
        parallel=parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    sys.exit(main())
