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


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _resolve_binaries() -> tuple[Path, Path]:
    """Return (nrflo_server, nrflo) absolute paths or die with a clear message."""
    server = shutil.which("nrflo_server")
    cli = shutil.which("nrflo")
    if not server or not cli:
        sys.exit(
            "manual_testing: nrflo_server or nrflo not on PATH. "
            "Run `make install` (or add ./be to PATH) first."
        )
    return Path(server), Path(cli)


@dataclass
class RunningServer:
    home: Path                  # NRFLO_HOME (also the DB dir)
    port: int                   # bound HTTP port on 127.0.0.1
    base_url: str               # http://127.0.0.1:<port>
    proc: subprocess.Popen      # the live server process
    log_path: Path              # captured stdout+stderr
    nrflo_cli: Path             # path to the agent CLI binary
    socket_path: Path           # NRFLO_SOCKET — short /tmp path, see start_server

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
        # Always remove the agent socket file — it lives outside NRFLO_HOME
        # (short /tmp path to dodge macOS's 104-byte AF_UNIX cap).
        try:
            self.socket_path.unlink(missing_ok=True)
        except OSError:
            pass
        if not keep_dir:
            shutil.rmtree(self.home, ignore_errors=True)
        else:
            print(f"[server] data dir kept at: {self.home}", flush=True)


def start_server(
    *,
    cli_label: str,
    extra_env: dict[str, str] | None = None,
) -> RunningServer:
    """Start a fresh nrflo_server. Caller MUST eventually call .stop().

    `extra_env` is merged into the child process environment after the
    standard NRFLO_* vars are set. Used by the api-mode runner to inject
    `ANTHROPIC_OAUTH_TOKEN` so the server resolves it via
    `apirun/provider/anthropic.ResolveAPIKey` step 4 (server env)."""
    server_bin, cli_bin = _resolve_binaries()

    home = Path(tempfile.mkdtemp(prefix=f"nrflo-manual-{cli_label}-"))
    port = _free_port()
    log_path = home / "server.log"

    # NRFLO_SOCKET: short /tmp path so multiple manual-test runs (different
    # provider+mode subprocesses, or different test invocations on the same
    # machine) get their own AF_UNIX endpoint. Two reasons not to rely on
    # the $NRFLO_HOME/agent.sock default:
    #   1. NRFLO_HOME sits under /var/folders/... (macOS tempfile) or a
    #      similarly long path; concatenating /agent.sock can flirt with
    #      the 104-byte AF_UNIX cap on macOS.
    #   2. Explicit-per-server isolation matches the Go integration harness
    #      (be/internal/integration/testenv_test.go) so neither parallel
    #      subprocesses nor a stale socket from a prior run can collide.
    socket_path = Path(
        f"/tmp/nrflo-manual-{cli_label}-{os.getpid()}.sock"
    )
    # Stale leftover from a crashed prior run would make BindListener fail.
    try:
        socket_path.unlink(missing_ok=True)
    except OSError:
        pass

    env = os.environ.copy()
    env["NRFLO_HOME"] = str(home)
    env["NRFLO_SOCKET"] = str(socket_path)
    # Make sure the spawned-agent processes can find the nrflo CLI.
    env["PATH"] = f"{cli_bin.parent}{os.pathsep}{env.get('PATH', '')}"
    if extra_env:
        env.update(extra_env)

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
        try:
            socket_path.unlink(missing_ok=True)
        except OSError:
            pass
        tail = log_path.read_text(errors="replace").splitlines()[-40:]
        sys.exit(
            "manual_testing: server did not become ready within 15s.\n"
            f"NRFLO_HOME={home}\n"
            f"NRFLO_SOCKET={socket_path}\n"
            "--- server.log tail ---\n" + "\n".join(tail)
        )

    return RunningServer(
        home=home,
        port=port,
        base_url=base_url,
        proc=proc,
        log_path=log_path,
        nrflo_cli=cli_bin,
        socket_path=socket_path,
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
