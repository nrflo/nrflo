#!/usr/bin/env python3
"""Run every provider in the manual-testing suite.

LAUNCH MANUALLY ONLY. Each sub-invocation spawns a real CLI against real
provider credentials. Missing binaries are SKIPPED. Exits non-zero if
any sub-invocation reports failure.

Grid: {claude,codex,opencode} × cli_interactive + script × native

Usage:
    python3 manual_testing/run_all.py                  # all providers
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

PROVIDERS = ["claude", "codex", "opencode", "script"]
BINARIES = {"claude": "claude", "codex": "codex", "opencode": "opencode",
            "script": "python3"}

# `script` is the synthetic provider for `execution_mode='script'` agents:
# no LLM, no provider CLI, just python3. It runs a separate scenario list
# (scenarios_script/), so it has no mode axis.
PROVIDER_SCRIPTS_NO_MODE = {"script"}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--provider", action="append", choices=PROVIDERS,
                    help="restrict to one or more providers (default: all)")
    # Default None → let each test_<provider>.py choose its own parallelism.
    ap.add_argument("--parallel", type=int, default=None)
    args = ap.parse_args()

    providers = args.provider or PROVIDERS

    overall = 0
    summary: list[tuple[str, int, float]] = []
    grid_start = time.monotonic()
    for provider in providers:
        if not shutil.which(BINARIES[provider]):
            print(f"\n========== {provider} — SKIPPED "
                  f"(binary not on PATH) ==========", flush=True)
            summary.append((provider, 0, 0.0))
            continue
        script = HERE / f"test_{provider}.py"
        mode_label = "native" if provider in PROVIDER_SCRIPTS_NO_MODE else "cli_interactive"
        print(f"\n========== {provider} × {mode_label} ==========", flush=True)
        t0 = time.monotonic()
        cmd = [sys.executable, str(script)]
        if args.parallel is not None:
            cmd.append(f"--parallel={args.parallel}")
        rc = subprocess.run(cmd, cwd=str(HERE)).returncode
        wall = time.monotonic() - t0
        summary.append((provider, rc, wall))
        if rc != 0:
            overall = rc
    grid_wall = time.monotonic() - grid_start

    print("\n========== aggregate ==========")
    print(f"  {'provider':10}  {'wall':>8}  result")
    for provider, rc, wall in summary:
        if wall == 0.0:
            verdict = "SKIPPED (binary missing)"
        else:
            verdict = "OK" if rc == 0 else f"FAILED (rc={rc})"
        print(f"  {provider:10}  {wall:7.2f}s  {verdict}")
    print(f"  --- grid wall: {grid_wall:.2f}s "
          f"({len([s for s in summary if s[2] > 0])} ran, "
          f"{len([s for s in summary if s[2] == 0])} skipped)")
    return overall


if __name__ == "__main__":
    sys.exit(main())
