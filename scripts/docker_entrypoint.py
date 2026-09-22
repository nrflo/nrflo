#!/usr/bin/env python3

import json
import os
import sys
import tempfile
from pathlib import Path


def merge_json_object(path: Path, updates: dict) -> None:
    if path.exists():
        with path.open(encoding="utf-8") as handle:
            config = json.load(handle)
        if not isinstance(config, dict):
            raise ValueError(f"{path} must contain a JSON object")
        mode = path.stat().st_mode & 0o777
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        config = {}
        mode = 0o600

    if all(config.get(key) == value for key, value in updates.items()):
        return

    config.update(updates)
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(config, handle, indent=2)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def initialize_claude_config(home: Path) -> None:
    merge_json_object(home / ".claude.json", {"hasCompletedOnboarding": True})
    merge_json_object(
        home / ".claude" / "settings.json",
        {"skipDangerousModePermissionPrompt": True},
    )


def main() -> None:
    if len(sys.argv) < 2:
        raise SystemExit("usage: docker_entrypoint.py COMMAND [ARG ...]")
    home = Path(os.environ.get("HOME", "/data"))
    initialize_claude_config(home)
    os.execvp(sys.argv[1], sys.argv[1:])


if __name__ == "__main__":
    main()
