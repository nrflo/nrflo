#!/usr/bin/env python3
"""Run every (provider × mode) combination of the manual-testing suite.

LAUNCH MANUALLY ONLY. Each sub-invocation spawns a real CLI against real
provider credentials. Missing binaries are SKIPPED. Exits non-zero if
any sub-invocation reports failure.

Usage:
    python3 manual_testing/run_all.py                  # all 3 × 2 = 6 combos
    python3 manual_testing/run_all.py --mode=cli       # cli only
    python3 manual_testing/run_all.py --provider=claude
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
import time
from pathlib import Path


HERE = Path(__file__).resolve().parent

PROVIDERS = ["claude", "codex", "opencode"]
MODES = ["cli", "cli-interactive"]
BINARIES = {"claude": "claude", "codex": "codex", "opencode": "opencode"}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--provider", action="append", choices=PROVIDERS,
                    help="restrict to one or more providers (default: all)")
    ap.add_argument("--mode", action="append", choices=MODES,
                    help="restrict to one or more modes (default: all)")
    # Default None → let each test_<provider>.py choose its own parallelism
    # (codex/cli-interactive runs narrower because of PTY+reasoning latency).
    ap.add_argument("--parallel", type=int, default=None)
    args = ap.parse_args()

    providers = args.provider or PROVIDERS
    modes = args.mode or MODES

    overall = 0
    summary: list[tuple[str, str, int, float]] = []
    grid_start = time.monotonic()
    for provider in providers:
        if not shutil.which(BINARIES[provider]):
            for mode in modes:
                print(f"\n========== {provider} × {mode} — SKIPPED "
                      f"(binary not on PATH) ==========", flush=True)
                summary.append((provider, mode, 0, 0.0))
            continue
        script = HERE / f"test_{provider}.py"
        for mode in modes:
            print(f"\n========== {provider} × {mode} ==========", flush=True)
            t0 = time.monotonic()
            cmd = [sys.executable, str(script), f"--mode={mode}"]
            if args.parallel is not None:
                cmd.append(f"--parallel={args.parallel}")
            rc = subprocess.run(cmd, cwd=str(HERE)).returncode
            wall = time.monotonic() - t0
            summary.append((provider, mode, rc, wall))
            if rc != 0:
                overall = rc
    grid_wall = time.monotonic() - grid_start

    print("\n========== aggregate ==========")
    print(f"  {'provider':10}  {'mode':18}  {'wall':>8}  result")
    for provider, mode, rc, wall in summary:
        if wall == 0.0:
            verdict = "SKIPPED (binary missing)"
        else:
            verdict = "OK" if rc == 0 else f"FAILED (rc={rc})"
        print(f"  {provider:10}  {mode:18}  {wall:7.2f}s  {verdict}")
    print(f"  --- grid wall: {grid_wall:.2f}s "
          f"({len([s for s in summary if s[3] > 0])} ran, "
          f"{len([s for s in summary if s[3] == 0])} skipped)")
    return overall


if __name__ == "__main__":
    sys.exit(main())
