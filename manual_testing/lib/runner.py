"""Manual-testing harness runner. `run_all` boots a fresh server for the
chosen provider+mode, logs in, runs every scenario from
`scenarios.ALL_SCENARIOS` (sequential or parallel), prints a verbose
timed log, and returns an exit code."""

from __future__ import annotations

import shutil
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import replace
from datetime import datetime
from typing import Callable

from . import api as api_mod
from . import server as server_mod
from .runtime import Ctx, Result, VALID_MODES


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
    provider: str,
    model: str,
    binary: str,
    mode: str = "cli",
    parallel: int = 1,
    only: list[str] | None = None,
    timeout: float | None = None,
) -> int:
    """Returns 0 (all PASS/SKIP), 1 (any FAIL), 2 (fatal interruption)."""
    if mode not in VALID_MODES:
        raise ValueError(f"invalid mode {mode!r}; want one of {VALID_MODES}")

    if timeout is not None:
        from . import runtime as _runtime
        _runtime.RUN_TIMEOUT_S = float(timeout)

    # Lazy import — avoids loading every scenario module on argparse errors.
    from scenarios import ALL_SCENARIOS  # type: ignore[import-not-found]

    if only:
        wanted = {n.strip() for n in only}
        filtered = [fn for fn in ALL_SCENARIOS
                    if any(fn.__module__.endswith(w) or w in fn.__module__
                           for w in wanted)]
        if not filtered:
            print(f"[{_ts()}] no scenarios matched --only={only}; "
                  f"have: {[fn.__module__.rsplit('.',1)[-1] for fn in ALL_SCENARIOS]}")
            return 1
        ALL_SCENARIOS = filtered

    label = f"{provider}/{mode}"
    bin_path = shutil.which(binary)
    if not bin_path:
        print(f"[{_ts()}] [{label}] SKIPPED — binary {binary!r} not on PATH")
        return 0
    parallel = max(1, parallel)
    _say(label, f"resolved {binary!r} → {bin_path}")
    _say(label, f"model={model}  mode={mode}  parallel={parallel}")

    boot_start = time.monotonic()
    _say(label, "booting nrflo_server …")
    srv = server_mod.start_server(cli_label=f"{provider}-{mode}")
    _say(label, f"server ready in {time.monotonic() - boot_start:.2f}s "
                f"at {srv.base_url} (NRFLO_HOME={srv.home})")
    client = api_mod.NrfloClient(srv.base_url)
    client.login()
    _say(label, "logged in as admin")
    base_ctx = Ctx(
        server=srv, client=client,
        provider=provider, model=model, binary=binary, mode=mode,
    )

    total = len(ALL_SCENARIOS)
    collected: list[TimedResult] = []
    fatal: str | None = None
    suite_start = time.monotonic()
    try:
        if parallel == 1:
            for i, fn in enumerate(ALL_SCENARIOS, start=1):
                collected.append(_run_one(fn, base_ctx,
                    index=i, total=total, label=label))
        else:
            with ThreadPoolExecutor(max_workers=parallel) as pool:
                futures = [
                    pool.submit(_run_one, fn, base_ctx,
                                index=i, total=total, label=label)
                    for i, fn in enumerate(ALL_SCENARIOS, start=1)
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
    if fatal:
        print(f"  fatal: {fatal}")
        return 2
    return 0 if fails == 0 else 1
