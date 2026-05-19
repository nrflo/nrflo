"""S46 — Observer agents: launch, read, mutate, scope enforcement, flag-off.

Tests handlers_observer.go:13 (POST /api/v1/observers) and :57 (GET),
spawn_observer.go:23, and handler_observer*.go socket dispatch.

Expected PASS:
  - workflow-scope observer can read (observer.workflow.show) and
    mutate (observer.workflow.trigger → new instance_id returned)
  - project-scope observer can read (observer.project.workflows) and
    mutate (observer.project.workflow.create → verifies via REST)
  - global-scope observer can read (observer.global.projects) and
    mutate (observer.global.project.create → project appears in REST list)
  - workflow-scope observer calling observer.global.projects returns
    an error containing 'permission denied'
  - POST /api/v1/observers returns 404 when experimental_observer_enabled=false
"""

from __future__ import annotations

import json
import socket as _socket
import tempfile

from lib.runtime import (
    Ctx, Result,
    make_project, next_id, resolve_model,
)

MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = "nrflo agent finished"


def _socket_call(ctx: Ctx, method: str, params: dict) -> tuple[object, object]:
    """Send a single JSON-RPC call to the Unix socket; return (result, error)."""
    payload = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}) + "\n"
    with _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM) as s:
        s.connect(str(ctx.server.socket_path))
        s.sendall(payload.encode())
        buf = b""
        while b"\n" not in buf:
            chunk = s.recv(4096)
            if not chunk:
                break
            buf += chunk
    resp = json.loads(buf.split(b"\n")[0])
    return resp.get("result"), resp.get("error")


def run(ctx: Ctx) -> Result:
    ctx.client.login()
    ctx.client.set_global_setting("experimental_observer_enabled", True)

    # Setup: project + workflow def + one agent def (self-terminates quickly).
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )

    obs_sessions: list[str] = []

    def _launch(scope: str, **kwargs: str) -> str:
        resp = ctx.client._request("POST", "/api/v1/observers", body={"scope": scope, **kwargs})
        sid = resp["session_id"]
        obs_sessions.append(sid)
        return sid

    wf_sid   = _launch("workflow", project_id=pid, workflow_id=wid)
    proj_sid = _launch("project",  project_id=pid)
    glob_sid = _launch("global")

    # --- workflow scope: read ---
    result, err = _socket_call(ctx, "observer.workflow.show", {
        "session_id": wf_sid, "project_id": pid, "workflow_id": wid,
    })
    if err or not result:
        return ("S46 observer", "FAIL", f"workflow.show failed: err={err}")

    # --- workflow scope: mutate (trigger new run) ---
    result, err = _socket_call(ctx, "observer.workflow.trigger", {
        "session_id": wf_sid, "project_id": pid, "workflow_id": wid,
        "instructions": "obs trigger", "scope_type": "project",
    })
    if err or not isinstance(result, dict) or not result.get("instance_id"):
        return ("S46 observer", "FAIL", f"workflow.trigger failed: err={err} result={result}")
    triggered_iid = result["instance_id"]

    # --- project scope: read ---
    result, err = _socket_call(ctx, "observer.project.workflows", {
        "session_id": proj_sid, "project_id": pid,
    })
    if err or not isinstance(result, list):
        return ("S46 observer", "FAIL", f"project.workflows failed: err={err}")
    if not any(w.get("id") == wid for w in result):
        return ("S46 observer", "FAIL", f"stub workflow {wid!r} not in project.workflows list")

    # --- project scope: mutate (create a new workflow def) ---
    stub_wid = next_id(ctx, "obs")
    result, err = _socket_call(ctx, "observer.project.workflow.create", {
        "session_id": proj_sid, "project_id": pid,
        "id": stub_wid, "scope_type": "project", "description": "obs",
    })
    if err or not result:
        return ("S46 observer", "FAIL", f"project.workflow.create failed: err={err}")
    wfs = ctx.client._request("GET", "/api/v1/workflows", project=pid)
    wf_ids = [w.get("id") for w in (wfs if isinstance(wfs, list) else [])]
    if stub_wid not in wf_ids:
        return ("S46 observer", "FAIL", f"created workflow {stub_wid!r} not in REST list: {wf_ids}")

    # --- global scope: read ---
    result, err = _socket_call(ctx, "observer.global.projects", {"session_id": glob_sid})
    if err or not isinstance(result, list):
        return ("S46 observer", "FAIL", f"global.projects failed: err={err}")
    if not any(p.get("id") == pid for p in result):
        return ("S46 observer", "FAIL", f"test project {pid!r} not in global.projects list")

    # --- global scope: mutate (create a new project) ---
    obs_pid = next_id(ctx, "obs-p")
    with tempfile.TemporaryDirectory() as tmp:
        result, err = _socket_call(ctx, "observer.global.project.create", {
            "session_id": glob_sid, "project_id": obs_pid, "root_path": tmp,
        })
    if err or not result:
        return ("S46 observer", "FAIL", f"global.project.create failed: err={err}")
    projects_resp = ctx.client._request("GET", "/api/v1/projects")
    proj_ids = [p.get("id") for p in projects_resp.get("projects", [])]
    if obs_pid not in proj_ids:
        return ("S46 observer", "FAIL", f"created project {obs_pid!r} not in REST list")

    # --- negative: workflow-scope observer cannot call global method ---
    _result, err = _socket_call(ctx, "observer.global.projects", {"session_id": wf_sid})
    if not err or "permission denied" not in str(err.get("message", "")):
        return ("S46 observer", "FAIL",
                f"expected 'permission denied', got err={err!r} result={_result!r}")

    # --- teardown: kill observer processes, remove created resources ---
    for sid in obs_sessions:
        try:
            ctx.client._request(
                "POST", f"/api/v1/projects/{pid}/workflow/kill-interactive",
                body={"workflow": wid, "session_id": sid},
                expect_status=200,
            )
        except Exception:
            pass
    for del_wid in (stub_wid,):
        try:
            ctx.client._request("DELETE", f"/api/v1/workflows/{del_wid}", project=pid)
        except Exception:
            pass
    try:
        ctx.client._request("DELETE", f"/api/v1/projects/{obs_pid}")
    except Exception:
        pass

    # --- flag-off 404 ---
    ctx.client.set_global_setting("experimental_observer_enabled", False)
    status, _ = ctx.client._request(
        "POST", "/api/v1/observers", body={"scope": "global"}, expect_status=404,
    )
    if status != 404:
        return ("S46 observer", "FAIL", f"expected 404 when flag off, got HTTP {status}")

    return ("S46 observer", "PASS",
            f"wf show+trigger({triggered_iid[:8]}), "
            f"proj workflows+create({stub_wid}), "
            f"glob projects+create({obs_pid}), "
            f"scope-deny OK, flag-off 404 OK")
