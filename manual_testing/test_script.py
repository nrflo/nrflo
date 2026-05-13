#!/usr/bin/env python3
"""Manual integration test for `execution_mode='script'` agents.

LAUNCH MANUALLY ONLY. No provider CLI / API credentials are needed —
the script backend spawns plain `python3` running stored Python that
drives the workflow via the embedded `nrflo_sdk` over the agent
socket. Server itself is the only external dependency.

Usage:
    python3 manual_testing/test_script.py
    python3 manual_testing/test_script.py --parallel=1
    python3 manual_testing/test_script.py --only=ps01,ps05
"""

from __future__ import annotations

import argparse
import sys

from lib.runner import run_scripts


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=5)
    ap.add_argument("--only", default=None,
                    help="comma-separated scenario substrings; e.g. ps01,ps05")
    ap.add_argument("--timeout", type=float, default=None,
                    help="per-scenario workflow-wait timeout in seconds")
    args = ap.parse_args()
    return run_scripts(
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    sys.exit(main())
