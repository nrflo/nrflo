#!/usr/bin/env python3
"""Manual integration test for the `gemini` CLI provider.

LAUNCH MANUALLY ONLY. Not wired into make test / CI. Spawns a real
`gemini` process against real Google credentials.

Usage:
    python3 manual_testing/test_gemini.py
    python3 manual_testing/test_gemini.py --parallel=1
"""

from __future__ import annotations

import argparse
import sys

from lib.runner import run_all


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=3)
    ap.add_argument("--model", default="gemini_flash_lite")
    ap.add_argument("--only", default=None,
                    help="comma-separated scenario substrings; e.g. s01,s07")
    ap.add_argument("--timeout", type=float, default=300.0,
                    help="per-scenario workflow-wait timeout in seconds")
    args = ap.parse_args()
    return run_all(
        provider="gemini",
        model=args.model,
        binary="gemini",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
    )


if __name__ == "__main__":
    sys.exit(main())
