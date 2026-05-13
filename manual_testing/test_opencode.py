#!/usr/bin/env python3
"""Manual integration test for the `opencode` CLI provider.

LAUNCH MANUALLY ONLY. Not wired into make test / CI. Spawns a real
`opencode` process against its configured provider credentials.

Usage:
    python3 manual_testing/test_opencode.py                  # mode=cli (default)
    python3 manual_testing/test_opencode.py --mode=cli-interactive
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
    ap.add_argument("--model", default="opencode_gpt54_mini_low")
    ap.add_argument("--only", default=None,
                    help="comma-separated scenario substrings; e.g. s01,s07")
    ap.add_argument("--timeout", type=float, default=None,
                    help="per-scenario workflow-wait timeout in seconds")
    args = ap.parse_args()
    return run_all(
        provider="opencode",
        model=args.model,
        binary="opencode",
        mode=args.mode,
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    sys.exit(main())
