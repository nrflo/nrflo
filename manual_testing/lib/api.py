"""Cookie-based REST client for the manual-testing harness. Talks to the
freshly-spawned nrflo_server as the seeded admin user."""

from __future__ import annotations

import http.cookiejar
import json
import mimetypes
import os
import urllib.error
import urllib.request
import uuid
from dataclasses import dataclass
from typing import Any


def _maybe_json(raw: str) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw


@dataclass
class APIError(Exception):
    status: int
    body: str
    method: str
    path: str

    def __str__(self) -> str:
        return f"{self.method} {self.path} -> {self.status}: {self.body[:300]}"


class NrfloClient:
    def __init__(self, base_url: str) -> None:
        self.base_url = base_url.rstrip("/")
        self._jar = http.cookiejar.CookieJar()
        self._opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self._jar)
        )
        # Process-wide default forwarded into every create_agent_def body.
        # The runner sets this to "cli_interactive" when mode == cli-interactive
        # so scenarios don't have to know about execution modes. Race-free
        # because each test_<provider>.py subprocess has exactly one mode and
        # sets this once before scenarios run.
        self.default_execution_mode: str | None = None

    # ---- raw transport -------------------------------------------------

    def _request(
        self,
        method: str,
        path: str,
        *,
        body: Any = None,
        project: str | None = None,
        expect_status: int | None = None,
        bearer: str | None = None,
        raw_body: bytes | None = None,
        content_type: str | None = None,
        use_cookies: bool = True,
    ) -> Any:
        """Issue a request. When `expect_status` is set, return
        `(status, decoded_body)` for *any* HTTP status and never raise —
        callers asserting negative paths (e.g. 409) use this.
        Otherwise non-2xx raises APIError.

        `bearer` sends `Authorization: Bearer <token>` and bypasses the
        cookie jar (used by service-token scenarios).
        `raw_body`/`content_type` are used for multipart uploads."""
        url = self.base_url + path
        data: bytes | None = None
        headers: dict[str, str] = {"Accept": "application/json"}
        if raw_body is not None:
            data = raw_body
            if content_type:
                headers["Content-Type"] = content_type
        elif body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if project:
            headers["X-Project"] = project
        if bearer:
            headers["Authorization"] = f"Bearer {bearer}"
        opener = self._opener if use_cookies else urllib.request.build_opener()
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with opener.open(req, timeout=30) as resp:
                raw = resp.read().decode("utf-8")
                status = resp.status
        except urllib.error.HTTPError as e:
            err_raw = e.read().decode("utf-8", errors="replace")
            if expect_status is not None:
                return e.code, _maybe_json(err_raw)
            raise APIError(
                status=e.code,
                body=err_raw,
                method=method,
                path=path,
            ) from None
        decoded = _maybe_json(raw)
        if expect_status is not None:
            return status, decoded
        return decoded

    # ---- auth ----------------------------------------------------------

    def login(self, email: str = "admin", password: str = "admin") -> None:
        self._request("POST", "/api/v1/auth/login",
                      body={"email": email, "password": password})

    # ---- global settings -----------------------------------------------

    def set_global_setting(self, key: str, value: Any) -> dict:
        """PATCH /api/v1/settings — used by the api-mode runner to flip
        `api_mode_enabled` on, and by scenarios that need to flip it off
        again to exercise the disabled-rejection path."""
        return self._request("PATCH", "/api/v1/settings", body={key: value})

    # ---- projects ------------------------------------------------------

    def create_project(self, project_id: str, *, root_path: str, name: str | None = None) -> dict:
        return self._request("POST", "/api/v1/projects", body={
            "id": project_id,
            "name": name or project_id,
            "root_path": root_path,
            "use_git_worktrees": False,
        })

    # ---- workflows -----------------------------------------------------

    def create_workflow(
        self,
        project_id: str,
        workflow_id: str,
        *,
        scope_type: str = "project",
        description: str = "",
        close_ticket_on_complete: bool = True,
        groups: list[str] | None = None,
        next_workflow_on_success: str = "",
    ) -> dict:
        body: dict[str, Any] = {
            "id": workflow_id,
            "description": description,
            "scope_type": scope_type,
            "close_ticket_on_complete": close_ticket_on_complete,
        }
        if groups:
            body["groups"] = groups
        if next_workflow_on_success:
            body["next_workflow_on_success"] = next_workflow_on_success
        return self._request("POST", "/api/v1/workflows", body=body, project=project_id)

    # ---- per-project settings + env vars + layer policies ---------------

    def patch_project(self, project_id: str, **fields: Any) -> dict:
        return self._request(
            "PATCH",
            f"/api/v1/projects/{project_id}",
            body=fields,
            project=project_id,
        )

    def put_project_env_var(self, project_id: str, name: str, value: str) -> dict:
        return self._request(
            "PUT",
            f"/api/v1/projects/{project_id}/env-vars/{name}",
            body={"value": value},
            project=project_id,
        )

    def set_layer_policy(
        self, project_id: str, workflow_id: str, layer: int, pass_policy: str,
    ) -> dict:
        return self._request(
            "PUT",
            f"/api/v1/workflows/{workflow_id}/layer-policies/{layer}",
            body={"pass_policy": pass_policy},
            project=project_id,
        )

    def create_agent_def(
        self,
        project_id: str,
        workflow_id: str,
        agent_id: str,
        *,
        model: str,
        prompt: str = "",
        layer: int = 0,
        timeout: int = 5,
        stall_start_timeout_sec: int | None = None,
        stall_running_timeout_sec: int | None = None,
        max_fail_restarts: int | None = None,
        restart_threshold: int | None = None,
        execution_mode: str | None = None,
        python_script_id: str | None = None,
        tools: str | None = None,
        validation_commands: list[str] | None = None,
    ) -> dict:
        body: dict[str, Any] = {
            "id": agent_id,
            "model": model,
            "timeout": timeout,
            "layer": layer,
        }
        # Script-mode agents must NOT carry a prompt (service rejects it).
        # Everything else: send prompt, empty string is fine for codepaths
        # that don't require it.
        effective_mode = execution_mode if execution_mode is not None else self.default_execution_mode
        if effective_mode != "script":
            body["prompt"] = prompt
        if python_script_id is not None:
            body["python_script_id"] = python_script_id
        if stall_start_timeout_sec is not None:
            body["stall_start_timeout_sec"] = stall_start_timeout_sec
        if stall_running_timeout_sec is not None:
            body["stall_running_timeout_sec"] = stall_running_timeout_sec
        if max_fail_restarts is not None:
            body["max_fail_restarts"] = max_fail_restarts
        if restart_threshold is not None:
            body["restart_threshold"] = restart_threshold
        if tools is not None:
            body["tools"] = tools
        if validation_commands is not None:
            body["validation_commands"] = validation_commands
        # Per-call override beats process default.
        mode = execution_mode if execution_mode is not None else self.default_execution_mode
        if mode is not None:
            body["execution_mode"] = mode
        return self._request(
            "POST",
            f"/api/v1/workflows/{workflow_id}/agents",
            body=body,
            project=project_id,
        )

    # ---- python scripts (project-scoped) ------------------------------

    def create_python_script(
        self, project_id: str, *, name: str, code: str = "",
        description: str = "", file_path: str | None = None,
        kind: str | None = None, tool_description: str | None = None,
        input_schema: str | None = None, timeout_sec: int | None = None,
    ) -> dict:
        body: dict[str, Any] = {"name": name, "code": code, "description": description}
        if file_path is not None:
            body["file_path"] = file_path
        if kind is not None:
            body["kind"] = kind
        if tool_description is not None:
            body["tool_description"] = tool_description
        if input_schema is not None:
            body["input_schema"] = input_schema
        if timeout_sec is not None:
            body["timeout_sec"] = timeout_sec
        return self._request(
            "POST",
            "/api/v1/python-scripts",
            body=body,
            project=project_id,
        )

    # ---- workflow runs (project scope) --------------------------------

    def run_project_workflow(
        self,
        project_id: str,
        workflow_id: str,
        *,
        instructions: str = "",
        endless_loop: bool = False,
        interactive: bool = False,
        plan_mode: bool = False,
        input_artifacts: list[dict] | None = None,
    ) -> dict:
        body: dict[str, Any] = {"workflow": workflow_id}
        if endless_loop:
            body["endless_loop"] = True
        else:
            body["instructions"] = instructions
        if interactive:
            body["interactive"] = True
        if plan_mode:
            body["plan_mode"] = True
        if input_artifacts:
            body["input_artifacts"] = input_artifacts
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/run",
            body=body,
            project=project_id,
        )

    def create_notification_channel(
        self, project_id: str, workflow_id: str, *,
        name: str, kind: str, config: dict,
        event_types: list[str] | None = None,
    ) -> dict:
        body: dict[str, Any] = {
            "name": name,
            "kind": kind,
            "config": config,
        }
        if event_types is not None:
            body["event_types"] = event_types
        return self._request(
            "POST",
            f"/api/v1/workflows/{workflow_id}/notification-channels",
            body=body,
            project=project_id,
        )

    def create_cli_model(
        self, *, id: str, cli_type: str, display_name: str,
        mapped_model: str, context_length: int,
        reasoning_effort: str = "",
    ) -> dict:
        return self._request(
            "POST",
            "/api/v1/cli-models",
            body={
                "id": id,
                "cli_type": cli_type,
                "display_name": display_name,
                "mapped_model": mapped_model,
                "reasoning_effort": reasoning_effort,
                "context_length": context_length,
            },
        )

    def take_control_project(
        self, project_id: str, *, workflow: str, session_id: str,
        instance_id: str,
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/take-control",
            body={
                "workflow": workflow,
                "session_id": session_id,
                "instance_id": instance_id,
            },
            project=project_id,
        )

    def exit_interactive_project(
        self, project_id: str, *, workflow: str, session_id: str,
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/exit-interactive",
            body={"workflow": workflow, "session_id": session_id},
            project=project_id,
        )

    def restart_project_workflow(
        self, project_id: str, *, workflow: str, session_id: str,
        instance_id: str,
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/restart",
            body={
                "workflow": workflow,
                "session_id": session_id,
                "instance_id": instance_id,
            },
            project=project_id,
        )

    def retry_failed_project(
        self, project_id: str, *, instance_id: str, workflow: str,
        session_id: str,
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/retry-failed",
            body={
                "instance_id": instance_id,
                "workflow": workflow,
                "session_id": session_id,
            },
            project=project_id,
        )

    def stop_endless_loop(
        self, project_id: str, *, instance_id: str, stop: bool = True,
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/stop-endless-loop",
            body={"instance_id": instance_id, "stop": stop},
            project=project_id,
        )

    # ---- workflow chains -----------------------------------------------

    def create_workflow_chain(
        self,
        project_id: str,
        chain_id: str,
        *,
        steps: list[dict],
        name: str | None = None,
    ) -> dict:
        return self._request(
            "POST",
            "/api/v1/workflow-chains",
            body={"id": chain_id, "name": name or chain_id, "steps": steps},
            project=project_id,
        )

    def start_workflow_chain_run(
        self,
        project_id: str,
        chain_id: str,
        *,
        instructions: str = "",
        triggered_by: str = "manual",
    ) -> dict:
        return self._request(
            "POST",
            f"/api/v1/workflow-chains/{chain_id}/runs",
            body={"instructions": instructions, "triggered_by": triggered_by},
            project=project_id,
        )

    def get_workflow_chain_run(
        self, project_id: str, chain_id: str, run_id: str,
    ) -> dict:
        return self._request(
            "GET",
            f"/api/v1/workflow-chains/{chain_id}/runs/{run_id}",
            project=project_id,
        )

    def get_project_workflow_state(
        self,
        project_id: str,
        *,
        workflow_id: str | None = None,
        instance_id: str | None = None,
    ) -> dict:
        q = []
        if workflow_id:
            q.append(f"workflow={workflow_id}")
        if instance_id:
            q.append(f"instance_id={instance_id}")
        suffix = ("?" + "&".join(q)) if q else ""
        return self._request(
            "GET",
            f"/api/v1/projects/{project_id}/workflow{suffix}",
            project=project_id,
        )

    def stop_project_workflow(self, project_id: str, *, instance_id: str | None = None) -> Any:
        body = {"instance_id": instance_id} if instance_id else {}
        return self._request(
            "POST",
            f"/api/v1/projects/{project_id}/workflow/stop",
            body=body,
            project=project_id,
        )

    # ---- tickets (only for S9) -----------------------------------------

    def create_ticket(self, project_id: str, *, ticket_id: str, title: str) -> dict:
        return self._request(
            "POST",
            "/api/v1/tickets",
            body={"id": ticket_id, "title": title, "created_by": "admin"},
            project=project_id,
        )

    def run_ticket_workflow(
        self,
        project_id: str,
        ticket_id: str,
        *,
        workflow_id: str,
        instructions: str = "",
        force: bool = True,
        interactive: bool = False,
        plan_mode: bool = False,
        expect_status: int | None = None,
    ) -> Any:
        body: dict[str, Any] = {"workflow": workflow_id, "force": force}
        if instructions:
            body["instructions"] = instructions
        if interactive:
            body["interactive"] = True
        if plan_mode:
            body["plan_mode"] = True
        return self._request(
            "POST",
            f"/api/v1/tickets/{ticket_id}/workflow/run",
            body=body,
            project=project_id,
            expect_status=expect_status,
        )

    def get_ticket_workflow_state(
        self,
        project_id: str,
        ticket_id: str,
        *,
        instance_id: str | None = None,
    ) -> dict:
        suffix = f"?instance_id={instance_id}" if instance_id else ""
        return self._request(
            "GET",
            f"/api/v1/tickets/{ticket_id}/workflow{suffix}",
            project=project_id,
        )

    # ---- workflow export / import -------------------------------------

    def export_workflow(self, project_id: str, workflow_id: str) -> dict:
        return self._request(
            "GET",
            f"/api/v1/workflows/{workflow_id}/export",
            project=project_id,
        )

    def import_workflow_check(self, project_id: str, bundle: dict) -> dict:
        return self._request(
            "POST",
            "/api/v1/workflows/import/check",
            body=bundle,
            project=project_id,
        )

    def import_workflow(
        self, project_id: str, bundle: dict, *, action: str = "overwrite",
    ) -> dict:
        return self._request(
            "POST",
            "/api/v1/workflows/import",
            body={"bundle": bundle, "action": action},
            project=project_id,
        )

    # ---- service tokens (admin-minted, project-scoped) -----------------

    def create_service_token(self, project_id: str, name: str) -> dict:
        return self._request(
            "POST",
            "/api/v1/service-tokens",
            body={"project_id": project_id, "name": name},
        )

    def bearer_get(
        self,
        path: str,
        *,
        token: str,
        project: str | None = None,
        expect_status: int | None = None,
    ) -> Any:
        """Issue GET with Authorization: Bearer <token> and no cookies. For
        service-token scope tests where the cookie jar would otherwise grant
        admin access."""
        return self._request(
            "GET", path,
            bearer=token, project=project,
            expect_status=expect_status, use_cookies=False,
        )

    # ---- artifacts ----------------------------------------------------

    def stage_artifact_upload(
        self, project_id: str, *, filename: str, data: bytes,
        content_type: str | None = None,
    ) -> dict:
        """POST /api/v1/artifact-uploads with a single file part. Returns
        ArtifactUploadResponse {upload_id, name, size_bytes, content_type}."""
        boundary = "----nrflo-manual-" + uuid.uuid4().hex
        ct = content_type or mimetypes.guess_type(filename)[0] or "application/octet-stream"
        parts: list[bytes] = []
        parts.append(f"--{boundary}\r\n".encode("utf-8"))
        parts.append(
            (f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
             f"Content-Type: {ct}\r\n\r\n").encode("utf-8")
        )
        parts.append(data)
        parts.append(b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode("utf-8"))
        body = b"".join(parts)
        return self._request(
            "POST", "/api/v1/artifact-uploads",
            project=project_id, raw_body=body,
            content_type=f"multipart/form-data; boundary={boundary}",
        )

    def list_artifacts(self, instance_id: str, project_id: str) -> list[dict]:
        return self._request(
            "GET",
            f"/api/v1/workflow-instances/{instance_id}/artifacts",
            project=project_id,
        )

    # ---- findings history ---------------------------------------------

    def findings_history(
        self, scope: str, scope_id: str, *,
        project_id: str, key: str | None = None,
    ) -> Any:
        q = [f"scope={scope}", f"scope_id={scope_id}"]
        if key:
            q.append(f"key={key}")
        return self._request(
            "GET",
            f"/api/v1/findings/history?" + "&".join(q),
            project=project_id,
        )
