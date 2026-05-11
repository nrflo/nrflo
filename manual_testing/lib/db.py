"""Read-only SQLite helpers for the manual-testing harness. Reads the same
nrflo.data the freshly-spawned server writes to."""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any


def _connect(home: Path) -> sqlite3.Connection:
    db_path = home / "nrflo.data"
    if not db_path.exists():
        raise RuntimeError(f"SQLite file not found at {db_path}")
    # uri=True + mode=ro keeps us honest about not mutating anything
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True, timeout=5)
    conn.row_factory = sqlite3.Row
    return conn


def agent_sessions_for_instance(home: Path, instance_id: str) -> list[dict[str, Any]]:
    with _connect(home) as c:
        rows = c.execute(
            """
            SELECT id, project_id, workflow_instance_id, phase, agent_type, model_id,
                   status, result, result_reason, pid, findings, context_left,
                   effective_mode, prompt, system_prompt, started_at, ended_at
            FROM agent_sessions
            WHERE workflow_instance_id = ?
            ORDER BY COALESCE(started_at, created_at) ASC
            """,
            (instance_id,),
        ).fetchall()
    return [_row_to_dict(r, json_keys=("findings",)) for r in rows]


def workflow_instances_for_workflow(
    home: Path, project_id: str, workflow_id: str,
) -> list[dict[str, Any]]:
    with _connect(home) as c:
        rows = c.execute(
            """
            SELECT id, project_id, workflow_id, scope_type, status, findings,
                   skip_tags, endless_loop, stop_endless_loop_after_iteration,
                   retry_count, created_at
            FROM workflow_instances
            WHERE project_id = ? AND workflow_id = ?
            ORDER BY created_at ASC
            """,
            (project_id, workflow_id),
        ).fetchall()
    return [_row_to_dict(r, json_keys=("findings", "skip_tags")) for r in rows]


def chain_run_steps(home: Path, chain_run_id: str) -> list[dict[str, Any]]:
    with _connect(home) as c:
        rows = c.execute(
            """
            SELECT id, chain_run_id, position, workflow_name, scope_type,
                   ticket_id, workflow_instance_id, instructions_used,
                   require_ticket_handoff, status
            FROM workflow_chain_run_steps
            WHERE chain_run_id = ?
            ORDER BY position ASC
            """,
            (chain_run_id,),
        ).fetchall()
    return [dict(r) for r in rows]


def workflow_instance(home: Path, instance_id: str) -> dict[str, Any] | None:
    with _connect(home) as c:
        row = c.execute(
            """
            SELECT id, project_id, ticket_id, workflow_id, scope_type, status,
                   findings, skip_tags, retry_count, created_at, updated_at
            FROM workflow_instances WHERE id = ?
            """,
            (instance_id,),
        ).fetchone()
    if row is None:
        return None
    return _row_to_dict(row, json_keys=("findings", "skip_tags"))


def agent_messages(home: Path, session_id: str) -> list[dict[str, Any]]:
    with _connect(home) as c:
        rows = c.execute(
            """
            SELECT seq, content, category, payload
            FROM agent_messages WHERE session_id = ? ORDER BY seq ASC
            """,
            (session_id,),
        ).fetchall()
    return [dict(r) for r in rows]


def project_findings(home: Path, project_id: str) -> dict[str, Any]:
    """Values are JSON-serialized on disk; decode so callers compare to plain
    Python values (`"alpha"` JSON → `"alpha"` str)."""
    with _connect(home) as c:
        rows = c.execute(
            "SELECT key, value FROM project_findings WHERE project_id = ?",
            (project_id,),
        ).fetchall()
    out: dict[str, Any] = {}
    for r in rows:
        v = r["value"]
        if isinstance(v, str) and v != "":
            try:
                v = json.loads(v)
            except json.JSONDecodeError:
                pass
        out[r["key"]] = v
    return out


def errors_for_project(home: Path, project_id: str) -> list[dict[str, Any]]:
    with _connect(home) as c:
        rows = c.execute(
            """
            SELECT id, error_type, instance_id, message, created_at
            FROM errors WHERE project_id = ? ORDER BY created_at ASC
            """,
            (project_id,),
        ).fetchall()
    return [dict(r) for r in rows]


def ticket(home: Path, project_id: str, ticket_id: str) -> dict[str, Any] | None:
    with _connect(home) as c:
        row = c.execute(
            """
            SELECT id, project_id, status, closed_at, close_reason
            FROM tickets WHERE project_id = ? AND id = ?
            """,
            (project_id, ticket_id),
        ).fetchone()
    return dict(row) if row else None


def _row_to_dict(row: sqlite3.Row, *, json_keys: tuple[str, ...]) -> dict[str, Any]:
    d = dict(row)
    for k in json_keys:
        v = d.get(k)
        if isinstance(v, str):
            if v == "":
                d[k] = [] if k == "skip_tags" else {}
                continue
            try:
                d[k] = json.loads(v)
            except json.JSONDecodeError:
                pass
    return d
