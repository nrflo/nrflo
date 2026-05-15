"""Version-probing helpers for the suite runner.

Each CLI is probed with `<binary> --version`. Gemini's CLI may not
implement `--version`; we fall back to "unknown" on any error."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path


_PROBE_TIMEOUT_S = 5.0


def probe(binary: str) -> str:
    """Return the first stdout line of `<binary> --version`, or 'unknown'."""
    path = shutil.which(binary)
    if not path:
        return "not on PATH"
    try:
        out = subprocess.run(
            [path, "--version"],
            capture_output=True, text=True, timeout=_PROBE_TIMEOUT_S,
        )
    except Exception:
        return "unknown"
    text = (out.stdout or out.stderr or "").strip().splitlines()
    return text[0] if text else "unknown"


def read_nrflo_version(repo_root: Path) -> str:
    f = repo_root / "VERSION"
    try:
        return f.read_text().strip()
    except OSError:
        return "unknown"
