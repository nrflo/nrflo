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
    ap.add_argument("--parallel", type=int, default=5)
    ap.add_argument("--model", default="codex_gpt54_mini_low")
    ap.add_argument("--only", default=None,
                    help="comma-separated scenario substrings; e.g. s01,s07")
    ap.add_argument("--timeout", type=float, default=None,
                    help="per-scenario workflow-wait timeout in seconds")
    args = ap.parse_args()
    return run_all(
        provider="codex",
        model=args.model,
        binary="codex",
        mode=args.mode,
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    sys.exit(main())
