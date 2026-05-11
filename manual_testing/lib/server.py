"""Spin up `nrflo_server serve` against a fresh NRFLO_HOME for a single
manual-testing session. Never used by automated tests."""

from __future__ import annotations

import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from dataclasses import dataclass
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _resolve_binaries() -> tuple[Path, Path]:
    """Return (nrflo_server, nrflo) absolute paths or die with a clear message."""
    server = REPO_ROOT / "be" / "nrflo_server"
    cli = REPO_ROOT / "be" / "nrflo"
    if not server.exists() or not cli.exists():
        sys.exit(
            "manual_testing: nrflo_server or nrflo binary missing from be/. "
            "Run `make build` first."
        )
    return server, cli


@dataclass
class RunningServer:
    home: Path                  # NRFLO_HOME (also the DB dir)
    port: int                   # bound HTTP port on 127.0.0.1
    base_url: str               # http://127.0.0.1:<port>
    proc: subprocess.Popen      # the live server process
    log_path: Path              # captured stdout+stderr
    nrflo_cli: Path             # path to the agent CLI binary

    def stop(self, *, keep_dir: bool = True) -> None:
        if self.proc.poll() is None:
            try:
                self.proc.send_signal(signal.SIGTERM)
                try:
                    self.proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    self.proc.kill()
                    self.proc.wait(timeout=2)
            except ProcessLookupError:
                pass
        if not keep_dir:
            shutil.rmtree(self.home, ignore_errors=True)
        else:
            print(f"[server] data dir kept at: {self.home}", flush=True)


def start_server(*, cli_label: str) -> RunningServer:
    """Start a fresh nrflo_server. Caller MUST eventually call .stop()."""
    server_bin, cli_bin = _resolve_binaries()

    home = Path(tempfile.mkdtemp(prefix=f"nrflo-manual-{cli_label}-"))
    port = _free_port()
    log_path = home / "server.log"

    env = os.environ.copy()
    env["NRFLO_HOME"] = str(home)
    # Make sure the spawned-agent processes can find the nrflo CLI.
    env["PATH"] = f"{cli_bin.parent}{os.pathsep}{env.get('PATH', '')}"

    log_fh = log_path.open("wb")
    proc = subprocess.Popen(
        [
            str(server_bin),
            "serve",
            "--host", "127.0.0.1",
            "--port", str(port),
            "--no-tray",
            "--insecure-cookies",
            "--mode", "cli",
        ],
        env=env,
        stdout=log_fh,
        stderr=subprocess.STDOUT,
        cwd=str(home),
    )

    base_url = f"http://127.0.0.1:{port}"
    if not _wait_ready(base_url, proc, timeout_s=15.0):
        proc.kill()
        log_fh.close()
        tail = log_path.read_text(errors="replace").splitlines()[-40:]
        sys.exit(
            "manual_testing: server did not become ready within 15s.\n"
            f"NRFLO_HOME={home}\n--- server.log tail ---\n" + "\n".join(tail)
        )

    return RunningServer(
        home=home,
        port=port,
        base_url=base_url,
        proc=proc,
        log_path=log_path,
        nrflo_cli=cli_bin,
    )


def _wait_ready(base_url: str, proc: subprocess.Popen, *, timeout_s: float) -> bool:
    """Server is ready when /api/v1/auth/me responds (401 is fine — it means
    routing + DB + session manager are wired up)."""
    deadline = time.monotonic() + timeout_s
    url = base_url + "/api/v1/auth/me"
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            return False
        try:
            with urllib.request.urlopen(url, timeout=1.0) as resp:
                if resp.status in (200, 401):
                    return True
        except urllib.error.HTTPError as e:
            if e.code in (200, 401):
                return True
        except (urllib.error.URLError, OSError):
            pass
        time.sleep(0.1)
    return False
