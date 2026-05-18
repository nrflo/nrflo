"""Manual-testing harness runner.

`run_all` boots a fresh server for the chosen provider, runs the given
scenario list (sequential or parallel), prints a verbose timed log,
optionally writes a JSON results file, and returns an exit code.

CLI providers pass `mode='cli_interactive'`. The python (script-mode)
provider passes `mode='script'` and `binary='python3'`."""

from __future__ import annotations

import json
import shutil
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import replace
from datetime import datetime
from pathlib import Path
from typing import Callable, Sequence

from . import api as api_mod
from . import server as server_mod
from .runtime import Ctx, Result


def _ts() -> str:
    return datetime.now().strftime("%H:%M:%S")


def _say(label: str, msg: str) -> None:
    print(f"[{_ts()}] [{label}] {msg}", flush=True)


TimedResult = tuple[int, Result, float]
MARK = {"PASS": "✓", "FAIL": "✗", "SKIP": "·"}


def _run_one(
    fn: Callable[[Ctx], Result],
    base_ctx: Ctx,
    *,
    index: int,
    total: int,
    label: str,
) -> TimedResult:
    name = fn.__module__.rsplit(".", 1)[-1]
    ctx = replace(base_ctx, scenario=name)
    _say(label, f"[{index}/{total}] >>> {name}")
    start = time.monotonic()
    try:
        result = fn(ctx)
    except Exception as e:
        result = (name, "FAIL", f"exception: {e!r}")
    dur = time.monotonic() - start
    mark = MARK.get(result[1], "?")
    _say(label, f"[{index}/{total}] {mark} {result[1]} {name} "
                f"({dur:5.2f}s) — {result[2]}")
    return (index, result, dur)


def run_all(
    *,
    scenarios: Sequence[Callable[[Ctx], Result]],
    provider: str,
    model: str,
    binary: str,
    mode: str,
    parallel: int = 1,
    only: list[str] | None = None,
    timeout: float | None = None,
    results_path: str | None = None,
) -> int:
    """Run `scenarios` for one provider on one fresh server.

    Returns 0 (all PASS/SKIP), 1 (any FAIL), 2 (fatal interruption)."""
    if timeout is not None:
        from . import runtime as _runtime
        _runtime.RUN_TIMEOUT_S = float(timeout)

    if only:
        wanted = {n.strip() for n in only}
        filtered = [fn for fn in scenarios
                    if any(fn.__module__.rsplit(".", 1)[-1].split("_", 1)[0] == w
                           or w in fn.__module__
                           for w in wanted)]
        if not filtered:
            print(f"[{_ts()}] no scenarios matched --only={only}; "
                  f"have: {[fn.__module__.rsplit('.',1)[-1] for fn in scenarios]}")
            return 1
        scenarios = filtered

    label = f"{provider}/{mode}"
    extra_env: dict[str, str] = {}
    if mode == "api":
        # api-mode runs in-process — no CLI binary needed. Source the
        # Anthropic OAuth token from the macOS Keychain (or env) and
        # SKIP the whole folder cleanly when none is reachable, just
        # like the CLI providers SKIP when their binary is missing.
        from .credentials import probe_oauth_token
        tok, reason = probe_oauth_token()
        if not tok:
            print(f"[{_ts()}] [{label}] SKIPPED — {reason}")
            if results_path:
                _write_results(results_path, [], skipped_reason=reason)
            return 0
        extra_env["ANTHROPIC_OAUTH_TOKEN"] = tok
        _say(label, f"resolved Anthropic OAuth token ({reason})")
    else:
        bin_path = shutil.which(binary)
        if not bin_path:
            print(f"[{_ts()}] [{label}] SKIPPED — binary {binary!r} not on PATH")
            if results_path:
                _write_results(results_path, [], skipped_reason=f"binary {binary!r} not on PATH")
            return 0
        _say(label, f"resolved {binary!r} → {bin_path}")
    parallel = max(1, parallel)
    _say(label, f"model={model}  mode={mode}  parallel={parallel}")

    boot_start = time.monotonic()
    _say(label, "booting nrflo_server …")
    srv = server_mod.start_server(cli_label=f"{provider}-{mode}", extra_env=extra_env)
    _say(label, f"server ready in {time.monotonic() - boot_start:.2f}s "
                f"at {srv.base_url} (NRFLO_HOME={srv.home})")
    client = api_mod.NrfloClient(srv.base_url)
    client.login()
    _say(label, "logged in as admin")
    client.default_execution_mode = mode
    if mode == "api":
        client.set_global_setting("api_mode_enabled", True)
        _say(label, "flipped api_mode_enabled=true")
    base_ctx = Ctx(
        server=srv, client=client,
        provider=provider, model=model, binary=binary, mode=mode,
    )

    total = len(scenarios)
    collected: list[TimedResult] = []
    fatal: str | None = None
    suite_start = time.monotonic()
    try:
        if parallel == 1:
            for i, fn in enumerate(scenarios, start=1):
                collected.append(_run_one(fn, base_ctx,
                    index=i, total=total, label=label))
        else:
            with ThreadPoolExecutor(max_workers=parallel) as pool:
                futures = [
                    pool.submit(_run_one, fn, base_ctx,
                                index=i, total=total, label=label)
                    for i, fn in enumerate(scenarios, start=1)
                ]
                for fut in as_completed(futures):
                    collected.append(fut.result())
    except KeyboardInterrupt:
        fatal = "KeyboardInterrupt"
    finally:
        suite_dur = time.monotonic() - suite_start
        _say(label, f"shutting server down (suite ran for {suite_dur:.2f}s)")
        srv.stop(keep_dir=True)

    collected.sort(key=lambda t: t[0])
    print()
    print(f"=== {label} results (parallel={parallel}) ===")
    fails = 0
    for _i, (name, verdict, details), dur in collected:
        print(f"  {verdict:4}  {name:36}  {dur:6.2f}s  {details}")
        if verdict == "FAIL":
            fails += 1
    total_in = sum(d for _i, _r, d in collected)
    print(f"  --- {len(collected)} scenarios, {fails} failed, "
          f"{total_in:.2f}s in-scenario sum, {suite_dur:.2f}s wall "
          f"(speedup ≈ {total_in / max(suite_dur, 0.001):.2f}x)")

    if results_path:
        rows = []
        for idx, (name, verdict, details), dur in collected:
            fn = scenarios[idx - 1]
            rows.append({
                "module": fn.__module__.rsplit(".", 1)[-1],
                "name": name,
                "verdict": verdict,
                "details": details,
                "duration_s": round(dur, 3),
            })
        _write_results(results_path, rows, wall_seconds=round(suite_dur, 3))

    if fatal:
        print(f"  fatal: {fatal}")
        return 2
    return 0 if fails == 0 else 1


def _write_results(
    path: str,
    rows: list[dict],
    *,
    wall_seconds: float | None = None,
    skipped_reason: str | None = None,
) -> None:
    out = {
        "rows": rows,
        "wall_seconds": wall_seconds,
        "skipped_reason": skipped_reason,
    }
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    Path(path).write_text(json.dumps(out, indent=2))
