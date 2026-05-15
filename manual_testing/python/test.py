#!/usr/bin/env python3
"""Manual integration test for python (script-mode) agents.

LAUNCH MANUALLY ONLY. No provider CLI / API credentials required —
the script backend spawns plain `python3` running stored Python that
drives the workflow via the embedded `nrflo_sdk` over the agent
socket. Scenarios run sequentially by default.

Usage:
    python3 manual_testing/python/test.py
    python3 manual_testing/python/test.py --only=P01,P05
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.runner import run_all  # noqa: E402

from python import ALL_SCENARIOS  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=1)
    ap.add_argument("--only", default=None)
    ap.add_argument("--timeout", type=float, default=300.0)
    ap.add_argument("--results", default=None)
    args = ap.parse_args()
    return run_all(
        scenarios=ALL_SCENARIOS,
        provider="python",
        model="haiku",
        binary="python3",
        mode="script",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
    )


if __name__ == "__main__":
    sys.exit(main())
