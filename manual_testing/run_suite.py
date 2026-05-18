#!/usr/bin/env python3
"""Run the full manual-testing suite.

All selected provider folders launch concurrently. Each subprocess
gets its own NRFLO_HOME (tempfile.mkdtemp) and NRFLO_SOCKET (short
/tmp path) via `lib/server.py`, so the servers don't share DB,
agent socket, or HTTP port. Scenarios within a single provider run
sequentially under that provider's server.

After everything finishes, results are aggregated, CLI versions probed,
and `/capabilities.md` overwritten at the repo root.

Usage:
    python3 manual_testing/run_suite.py
    python3 manual_testing/run_suite.py --only=engine,claude
    python3 manual_testing/run_suite.py --timeout=600
    python3 manual_testing/run_suite.py --sequential   # one provider at a time
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import shutil
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent

sys.path.insert(0, str(HERE))

from lib import versions as ver_mod  # noqa: E402


PROVIDERS: list[tuple[str, str]] = [
    # (provider folder, binary name)
    # All run concurrently by default — each subprocess gets its own
    # NRFLO_HOME + NRFLO_SOCKET via lib/server.py so server-side state
    # is fully isolated.
    ("engine", "claude"),
    ("claude", "claude"),
    ("codex", "codex"),
    ("gemini", "gemini"),
    ("opencode", "opencode"),
    ("python", "python3"),
    # `api` exercises execution_mode='api' in-process; no binary
    # required, but we pass 'claude' so the existing PATH probe in
    # _launch logs honestly. The runner SKIPs the whole folder when no
    # Anthropic OAuth token is reachable (see lib/credentials.py).
    ("api", "claude"),
]


def _ts() -> str:
    return dt.datetime.now().strftime("%H:%M:%S")


def _say(msg: str) -> None:
    print(f"[{_ts()}] [suite] {msg}", flush=True)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--only", default=None,
        help="comma-separated provider subset (e.g. claude,python)",
    )
    ap.add_argument("--timeout", type=float, default=300.0,
                    help="per-scenario timeout in seconds")
    ap.add_argument("--sequential", action="store_true",
                    help="run provider folders one at a time (default: all in parallel)")
    args = ap.parse_args()

    wanted = {p.strip() for p in args.only.split(",")} if args.only else None
    selected = [(p, b) for (p, b) in PROVIDERS if wanted is None or p in wanted]
    if not selected:
        print(f"no providers matched --only={args.only}; "
              f"have: {[p for p, _ in PROVIDERS]}")
        return 1

    run_dir = Path("/tmp") / f"nrflo-suite-{int(time.time())}"
    run_dir.mkdir(parents=True, exist_ok=True)
    _say(f"run dir: {run_dir}")

    def _launch(provider: str, binary: str) -> tuple[str, str, subprocess.Popen, Path, Path]:
        if not shutil.which(binary):
            _say(f"{provider}: binary {binary!r} not on PATH — will skip")
            # We still spawn it so the test.py records the SKIP in JSON.
        test_py = HERE / provider / "test.py"
        results_json = run_dir / f"{provider}.json"
        log_path = run_dir / f"{provider}.log"
        log_fh = open(log_path, "w")
        cmd = [
            sys.executable, str(test_py),
            "--parallel=1",
            f"--timeout={args.timeout}",
            f"--results={results_json}",
        ]
        _say(f"launch {provider}: {' '.join(cmd)}")
        p = subprocess.Popen(cmd, stdout=log_fh, stderr=subprocess.STDOUT)
        return (provider, binary, p, results_json, log_path)

    suite_start = time.monotonic()
    procs: list[tuple[str, str, subprocess.Popen, Path, Path]] = []
    exit_codes: dict[str, int] = {}

    if args.sequential:
        # One provider at a time. Mostly useful when an interactive
        # debugger is attached or when an external rate-limit forces
        # serialization.
        for provider, binary in selected:
            rec = _launch(provider, binary)
            procs.append(rec)
            rc = rec[2].wait()
            exit_codes[provider] = rc
            _say(f"{provider} exit={rc} ({rec[4]})")
    else:
        # Default: launch every provider concurrently. Each subprocess
        # spawns its own nrflo_server with an isolated NRFLO_HOME +
        # NRFLO_SOCKET (see lib/server.py.start_server), so the servers
        # don't share DB rows, agent sockets, or HTTP ports.
        for provider, binary in selected:
            procs.append(_launch(provider, binary))
        for provider, _binary, p, _json, log_path in procs:
            rc = p.wait()
            exit_codes[provider] = rc
            _say(f"{provider} exit={rc} ({log_path})")

    suite_wall = time.monotonic() - suite_start
    _say(f"all providers finished in {suite_wall:.2f}s")

    aggregated: dict[str, dict] = {}
    for provider, _binary, _p, json_path, _log in procs:
        if json_path.exists():
            try:
                aggregated[provider] = json.loads(json_path.read_text())
            except Exception as e:
                aggregated[provider] = {"rows": [], "wall_seconds": None,
                                        "load_error": str(e)}
        else:
            aggregated[provider] = {"rows": [], "wall_seconds": None,
                                    "load_error": "results file missing"}

    versions = {
        "nrflo": ver_mod.read_nrflo_version(REPO_ROOT),
        "claude": ver_mod.probe("claude"),
        "codex": ver_mod.probe("codex"),
        "gemini": ver_mod.probe("gemini"),
        "opencode": ver_mod.probe("opencode"),
        "python": ver_mod.probe("python3"),
    }

    write_capabilities(
        path=REPO_ROOT / "capabilities.md",
        aggregated=aggregated,
        versions=versions,
        suite_wall=suite_wall,
        selected_providers=[p for p, _ in selected],
        timestamp_utc=dt.datetime.now(dt.timezone.utc),
    )
    _say(f"wrote {REPO_ROOT / 'capabilities.md'}")

    print()
    print("=== suite summary ===")
    failed_any = False
    for provider, _binary, _p, _json, _log in procs:
        rc = exit_codes[provider]
        rows = aggregated[provider].get("rows", [])
        wall = aggregated[provider].get("wall_seconds")
        pass_n = sum(1 for r in rows if r.get("verdict") == "PASS")
        fail_n = sum(1 for r in rows if r.get("verdict") == "FAIL")
        skip_n = sum(1 for r in rows if r.get("verdict") == "SKIP")
        wall_s = f"{wall:.2f}s" if isinstance(wall, (int, float)) else "—"
        verdict = "OK" if rc == 0 else f"FAIL(rc={rc})"
        print(f"  {provider:10}  {pass_n:3} pass  {fail_n:3} fail  "
              f"{skip_n:3} skip  {wall_s:>8}  {verdict}")
        if rc != 0 or fail_n:
            failed_any = True
    print(f"  --- grid wall: {suite_wall:.2f}s")
    return 1 if failed_any else 0


def write_capabilities(
    *,
    path: Path,
    aggregated: dict[str, dict],
    versions: dict[str, str],
    suite_wall: float,
    selected_providers: list[str],
    timestamp_utc: dt.datetime,
) -> None:
    all_providers = [p for p, _ in PROVIDERS]
    scenario_ids: list[str] = []
    seen: set[str] = set()
    for provider in all_providers:
        for r in aggregated.get(provider, {}).get("rows", []):
            mod = r.get("module") or ""
            sid = mod.split("_", 1)[0] if "_" in mod else mod
            if sid and sid not in seen:
                seen.add(sid)
                scenario_ids.append(sid)
    scenario_ids.sort(key=_sid_sort_key)

    descriptions = _load_suite_descriptions()

    cell_by_provider: dict[str, dict[str, str]] = {}
    for provider in all_providers:
        cell_by_provider[provider] = {}
        rows = aggregated.get(provider, {}).get("rows", [])
        for r in rows:
            mod = r.get("module") or ""
            sid = mod.split("_", 1)[0] if "_" in mod else mod
            v = r.get("verdict")
            cell_by_provider[provider][sid] = {
                "PASS": "✅", "FAIL": "❌", "SKIP": "⊘"
            }.get(v, "?")

    ts = timestamp_utc.strftime("%Y-%m-%dT%H:%M:%SZ")
    lines: list[str] = []
    lines.append("# Provider Capability Matrix")
    lines.append("")
    lines.append(f"_Generated by `python3 manual_testing/run_suite.py` at "
                 f"{ts} (wall {suite_wall:.1f}s, providers: "
                 f"{', '.join(selected_providers)})._")
    lines.append("")
    lines.append("Edit `manual_testing/suite.md` to change the scenario "
                 "catalogue. This file is overwritten on every suite run.")
    lines.append("")
    lines.append("## Versions")
    lines.append("")
    lines.append("| Component | Version |")
    lines.append("|-----------|---------|")
    lines.append(f"| nrflo     | `{versions.get('nrflo','unknown')}` |")
    for prov in ["claude", "codex", "gemini", "opencode", "python"]:
        lines.append(f"| {prov:8}  | `{versions.get(prov,'unknown')}` |")
    lines.append("")
    lines.append("## Scenario results")
    lines.append("")
    lines.append("Legend: ✅ PASS · ❌ FAIL · ⊘ SKIP · — not applicable.")
    lines.append("")
    header = "| Scenario | " + " | ".join(all_providers) + " |"
    sep = "|" + "----------|" + "|".join(["------"] * len(all_providers)) + "|"
    lines.append(header)
    lines.append(sep)
    for sid in scenario_ids:
        desc = descriptions.get(sid, "")
        label = f"{sid} {desc}".strip() if desc else sid
        row = f"| {label} |"
        for prov in all_providers:
            cell = cell_by_provider.get(prov, {}).get(sid, "—")
            row += f" {cell} |"
        lines.append(row)
    lines.append("")

    lines.append("## Per-provider wall time")
    lines.append("")
    lines.append("| Provider | wall (s) | PASS | FAIL | SKIP |")
    lines.append("|----------|----------|------|------|------|")
    for prov in all_providers:
        agg = aggregated.get(prov, {})
        rows = agg.get("rows", [])
        wall = agg.get("wall_seconds")
        if wall is None:
            wall_s = "—"
        else:
            wall_s = f"{wall:.1f}"
        pass_n = sum(1 for r in rows if r.get("verdict") == "PASS")
        fail_n = sum(1 for r in rows if r.get("verdict") == "FAIL")
        skip_n = sum(1 for r in rows if r.get("verdict") == "SKIP")
        skip_reason = agg.get("skipped_reason")
        if skip_reason:
            lines.append(f"| {prov} | — | — | — | — _(skipped: {skip_reason})_ |")
        else:
            lines.append(f"| {prov} | {wall_s} | {pass_n} | {fail_n} | {skip_n} |")
    lines.append("")
    path.write_text("\n".join(lines) + "\n")


def _sid_sort_key(sid: str) -> tuple[int, int]:
    """Sort s* before P*; numeric within each prefix."""
    if not sid:
        return (9, 9999)
    prefix = sid[0]
    rank = 0 if prefix.lower() == "s" else 1
    try:
        n = int(sid[1:])
    except ValueError:
        n = 9999
    return (rank, n)


def _load_suite_descriptions() -> dict[str, str]:
    """Parse `manual_testing/suite.md` to map scenario id → description."""
    path = HERE / "suite.md"
    out: dict[str, str] = {}
    try:
        text = path.read_text()
    except OSError:
        return out
    for line in text.splitlines():
        if not line.startswith("| s") and not line.startswith("| P"):
            continue
        parts = [p.strip() for p in line.strip("|").split("|")]
        if len(parts) < 2:
            continue
        sid = parts[0]
        desc = parts[1]
        if sid.startswith(("s", "P")) and any(c.isdigit() for c in sid):
            out[sid] = desc
    return out


if __name__ == "__main__":
    sys.exit(main())
