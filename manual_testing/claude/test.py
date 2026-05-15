#!/usr/bin/env python3
"""Manual integration test for the `claude` CLI provider.

LAUNCH MANUALLY ONLY. Spawns a real `claude` process against real
Anthropic credentials. Scenarios run sequentially by default.

Usage:
    python3 manual_testing/claude/test.py
    python3 manual_testing/claude/test.py --only=s01,s07
    python3 manual_testing/claude/test.py --model=sonnet
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.runner import run_all  # noqa: E402

from claude import ALL_SCENARIOS  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=1)
    ap.add_argument("--model", default="haiku")
    ap.add_argument("--only", default=None)
    ap.add_argument("--timeout", type=float, default=300.0)
    ap.add_argument("--results", default=None,
                    help="optional JSON file path for per-scenario results")
    args = ap.parse_args()
    return run_all(
        scenarios=ALL_SCENARIOS,
        provider="claude",
        model=args.model,
        binary="claude",
        mode="cli_interactive",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
    )


if __name__ == "__main__":
    sys.exit(main())
