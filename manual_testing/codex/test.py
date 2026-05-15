#!/usr/bin/env python3
"""Manual integration test for the `codex` CLI provider.

LAUNCH MANUALLY ONLY. Spawns a real `codex` process against real OpenAI
credentials. Scenarios run sequentially by default.

Usage:
    python3 manual_testing/codex/test.py
    python3 manual_testing/codex/test.py --only=s01,s07
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.runner import run_all  # noqa: E402

from codex import ALL_SCENARIOS  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=1)
    ap.add_argument("--model", default="codex_gpt54_mini_low")
    ap.add_argument("--only", default=None)
    ap.add_argument("--timeout", type=float, default=300.0)
    ap.add_argument("--results", default=None)
    args = ap.parse_args()
    return run_all(
        scenarios=ALL_SCENARIOS,
        provider="codex",
        model=args.model,
        binary="codex",
        mode="cli_interactive",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
    )


if __name__ == "__main__":
    sys.exit(main())
