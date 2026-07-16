#!/usr/bin/env python3
"""Manual integration test for execution_mode='api' (OpenAI provider).

LAUNCH MANUALLY ONLY. Runs `apirun.Runner` in-process against a real
OpenAI Responses endpoint authenticated by the API key resolved from
the environment (`lib/credentials.probe_openai_key()`). When neither
OPENAI_API_KEY nor CODEX_API_KEY is set, every scenario SKIPs cleanly.

Usage:
    python3 manual_testing/openai_api/test.py
    python3 manual_testing/openai_api/test.py --only=A01,O01
    python3 manual_testing/openai_api/test.py --model=gpt-5.4
"""

from __future__ import annotations

import argparse
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from lib.runner import run_all  # noqa: E402

from openai_api import ALL_SCENARIOS  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--parallel", type=int, default=1)
    ap.add_argument("--model", default="gpt-5.4")
    ap.add_argument("--only", default=None)
    ap.add_argument("--timeout", type=float, default=300.0)
    ap.add_argument("--results", default=None,
                    help="optional JSON file path for per-scenario results")
    args = ap.parse_args()
    return run_all(
        scenarios=ALL_SCENARIOS,
        provider="openai_api",
        model=args.model,
        # api-mode runs in-process — binary is unused.
        binary="python3",
        mode="api",
        api_credentials="openai",
        parallel=args.parallel,
        only=args.only.split(",") if args.only else None,
        timeout=args.timeout,
        results_path=args.results,
    )


if __name__ == "__main__":
    sys.exit(main())
