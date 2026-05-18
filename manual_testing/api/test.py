#!/usr/bin/env python3
"""Manual integration test for execution_mode='api' (claude provider).

LAUNCH MANUALLY ONLY. Runs `apirun.Runner` in-process against a real
Anthropic endpoint authenticated by the OAuth bearer token resolved from
the macOS Keychain (`lib/credentials.py`). When no token is reachable
every scenario SKIPs cleanly.

Usage:
    python3 manual_testing/api/test.py
    python3 manual_testing/api/test.py --only=A01,A06
    python3 manual_testing/api/test.py --model=haiku_api
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.runner import run_all  # noqa: E402

from api import ALL_SCENARIOS  # noqa: E402


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
        provider="api",
        model=args.model,
        # api-mode runs in-process — the binary is unused. We pass
        # 'claude' so any debug log that mentions the underlying provider
        # is honest about which Anthropic surface this hits.
        binary="claude",
        mode="api",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
    )


if __name__ == "__main__":
    sys.exit(main())
