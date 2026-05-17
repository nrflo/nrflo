"""nrflo Python SDK — persistent Unix-socket client for script-mode agents.

Auto-populates session_id, instance_id, project, and trx from env vars set by
the spawner (NRF_SESSION_ID, NRF_WORKFLOW_INSTANCE_ID, NRFLO_PROJECT, NRF_TRX).
Socket path defaults to $NRFLO_HOME/agent.sock (override: NRFLO_SOCKET).

Usage:
    import sys
    sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
    import nrflo_sdk
    c = nrflo_sdk.client()
    c.findings.add("result", "done")
    c.agent.finished()
"""

import base64
import json
import os
import socket
import time
import uuid


class NrfloError(Exception):
    """Raised when the server returns an error response."""

    def __init__(self, code: int, message: str):
        super().__init__(f"[{code}] {message}")
        self.code = code
        self.message = message


class _Connection:
    """Persistent Unix-socket connection; retries on broken pipe up to 1 s."""

    def __init__(self, path: str):
        self._path = path
        self._sock = None
        self._file = None

    def _connect(self):
        if not os.path.exists(self._path):
            raise NrfloError(0, f"socket not found: {self._path} — is nrflo_server running?")
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        s.settimeout(300)
        s.connect(self._path)
        self._sock = s
        self._file = s.makefile("rb")

    def _close_sock(self):
        for obj in (self._file, self._sock):
            try:
                if obj is not None:
                    obj.close()
            except Exception:
                pass
        self._sock = None
        self._file = None

    def send(self, req: dict) -> dict:
        data = json.dumps(req).encode() + b"\n"
        deadline = time.monotonic() + 1.0
        attempt = 0
        while True:
            try:
                if self._sock is None:
                    self._connect()
                self._sock.sendall(data)
                line = self._file.readline()
                if not line:
                    raise BrokenPipeError("empty response")
                return json.loads(line)
            except (BrokenPipeError, ConnectionResetError, OSError):
                self._close_sock()
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise NrfloError(0, "connection lost; retry window expired")
                attempt += 1
                time.sleep(min(0.1 * attempt, remaining))

    def close(self):
        self._close_sock()


def _check(resp: dict) -> dict:
    """Raise NrfloError if response contains an error; otherwise return result."""
    err = resp.get("error")
    if err:
        raise NrfloError(err.get("code", 0), err.get("message", "unknown error"))
    return resp.get("result") or {}


class _Findings:
    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._iid = iid
        self._proj = proj
        self._trx = trx

    def _call(self, action: str, extra: dict) -> dict:
        params = {"session_id": self._sid, "instance_id": self._iid}
        params.update(extra)
        return _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": f"findings.{action}",
            "project": self._proj, "trx": self._trx, "params": params,
        }))

    def add(self, key: str, value: str):
        self._call("add", {"key": key, "value": value})

    def add_bulk(self, kv: dict):
        self._call("add-bulk", {"key_values": kv})

    def get(self, agent_type: str = None, *, key: str = None, keys: list = None, layer: int = None) -> dict:
        if agent_type is not None and layer is not None:
            raise ValueError("agent_type and layer are mutually exclusive")
        extra = {}
        if agent_type is not None:
            extra["agent_type"] = agent_type
        if key is not None:
            extra["key"] = key
        if keys is not None:
            extra["keys"] = keys
        if layer is not None:
            extra["layer"] = layer
        return self._call("get", extra)

    def append(self, key: str, value: str):
        self._call("append", {"key": key, "value": value})

    def append_bulk(self, kv: dict):
        self._call("append-bulk", {"key_values": kv})

    def delete(self, *keys: str):
        self._call("delete", {"keys": list(keys)})


class _ProjectFindings:
    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._iid = iid
        self._proj = proj
        self._trx = trx

    def _call(self, action: str, extra: dict) -> dict:
        params = {"session_id": self._sid, "instance_id": self._iid}
        params.update(extra)
        return _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": f"project_findings.{action}",
            "project": self._proj, "trx": self._trx, "params": params,
        }))

    def add(self, key: str, value: str):
        self._call("add", {"key": key, "value": value})

    def add_bulk(self, kv: dict):
        self._call("add-bulk", {"key_values": kv})

    def get(self, key: str = None, *, keys: list = None) -> dict:
        extra = {}
        if key:
            extra["key"] = key
        if keys:
            extra["keys"] = keys
        return self._call("get", extra)

    def append(self, key: str, value: str):
        self._call("append", {"key": key, "value": value})

    def append_bulk(self, kv: dict):
        self._call("append-bulk", {"key_values": kv})

    def delete(self, *keys: str):
        self._call("delete", {"keys": list(keys)})


class _Artifacts:
    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._proj = proj
        self._trx = trx

    def _call(self, action: str, extra: dict) -> dict:
        params = {"session_id": self._sid}
        params.update(extra)
        return _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": f"artifact.{action}",
            "project": self._proj, "trx": self._trx, "params": params,
        }))

    def add(self, name: str, content, content_type: str = None) -> dict:
        if isinstance(content, str):
            data = content.encode("utf-8")
        elif isinstance(content, (bytes, bytearray)):
            data = bytes(content)
        else:
            raise TypeError(f"content must be str or bytes, got {type(content).__name__}")
        if len(data) > 32 * 1024 * 1024:
            raise NrfloError(0, "artifact too large: max 32 MiB")
        extra = {"name": name, "content_b64": base64.b64encode(data).decode("ascii")}
        if content_type:
            extra["content_type"] = content_type
        return self._call("add", extra)

    def list(self) -> list:
        params = {"session_id": self._sid}
        resp = self._conn.send({
            "id": str(uuid.uuid4()), "method": "artifact.list",
            "project": self._proj, "trx": self._trx, "params": params,
        })
        err = resp.get("error")
        if err:
            raise NrfloError(err.get("code", 0), err.get("message", "unknown error"))
        return resp.get("result", [])

    def get(self, name: str) -> str:
        result = self._call("get", {"name": name})
        return result.get("path", "")


class _Agent:
    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._iid = iid
        self._proj = proj
        self._trx = trx

    def _call(self, action: str, extra: dict = None):
        params = {"session_id": self._sid, "instance_id": self._iid}
        if extra:
            params.update(extra)
        _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": f"agent.{action}",
            "project": self._proj, "trx": self._trx, "params": params,
        }))

    def finished(self):
        self._call("finished")

    def fail(self, reason: str = ""):
        self._call("fail", {"reason": reason} if reason else None)

    def continue_(self):
        self._call("continue")

    def callback(self, level: int):
        self._call("callback", {"level": level})

    def chain_next_ticket(self, ticket_id: str):
        self._call("chain_next_ticket", {"ticket_id": ticket_id})


class _Notification:
    """Parsed NRFLO_NOTIFY_PAYLOAD_JSON payload for notification scripts."""

    def __init__(self):
        raw_env = os.environ.get("NRFLO_NOTIFY_PAYLOAD_JSON")
        if not raw_env:
            raise NrfloError(0, "no notification payload in env (NRFLO_NOTIFY_PAYLOAD_JSON unset)")
        try:
            self._raw = json.loads(raw_env)
        except json.JSONDecodeError as e:
            raise NrfloError(0, f"invalid JSON in NRFLO_NOTIFY_PAYLOAD_JSON: {e}")

    @property
    def event_type(self) -> str:
        return self._raw.get("event_type", "")

    @property
    def project_id(self) -> str:
        return self._raw.get("project_id", "")

    @property
    def project_name(self) -> str:
        return self._raw.get("project_name", "")

    @property
    def workflow(self) -> str:
        return self._raw.get("workflow", "")

    @property
    def instance_id(self) -> str:
        return self._raw.get("instance_id", "")

    @property
    def ticket_id(self) -> str:
        return self._raw.get("ticket_id", "")

    @property
    def ticket_name(self) -> str:
        return self._raw.get("ticket_name", "")

    @property
    def agent_type(self) -> str:
        return self._raw.get("agent_type", "")

    @property
    def reason(self) -> str:
        return self._raw.get("reason", "")

    @property
    def summary(self) -> str:
        return self._raw.get("workflow_final_result", "")

    @property
    def raw(self) -> dict:
        return self._raw


class Client:
    """nrflo script-mode client. Obtain via nrflo_sdk.client()."""

    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._iid = iid
        self._proj = proj
        self._trx = trx
        self._ctx_cache = None
        self._notification_cache = None
        self.findings = _Findings(conn, sid, iid, proj, trx)
        self.project_findings = _ProjectFindings(conn, sid, iid, proj, trx)
        self.agent = _Agent(conn, sid, iid, proj, trx)
        self.artifacts = _Artifacts(conn, sid, iid, proj, trx)

    def context(self, refresh: bool = False) -> dict:
        """Return the auto-injectable variable dict for this session (cached)."""
        if self._ctx_cache is None or refresh:
            self._ctx_cache = _check(self._conn.send({
                "id": str(uuid.uuid4()), "method": "script.context",
                "trx": self._trx, "params": {"session_id": self._sid},
            }))
        return self._ctx_cache

    def user_instructions(self) -> str:
        return self.context().get("user_instructions", "")

    def callback_info(self):
        """Return callback dict {instructions, from_agent, level} or None."""
        return self.context().get("callback")

    def previous_data(self) -> str:
        return self.context().get("previous_data", "")

    def skip(self, tag: str):
        _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": "workflow.skip",
            "trx": self._trx,
            "params": {"instance_id": self._iid, "tag": tag},
        }))

    def log(self, type: str = "text", message: str = "", payload=None):
        params = {"session_id": self._sid, "type": type or "text", "message": message}
        if payload is not None:
            params["payload"] = payload
        _check(self._conn.send({
            "id": str(uuid.uuid4()), "method": "agent.log",
            "trx": self._trx, "params": params,
        }))

    def notification(self) -> _Notification:
        """Return parsed NRFLO_NOTIFY_PAYLOAD_JSON (cached)."""
        if self._notification_cache is None:
            self._notification_cache = _Notification()
        return self._notification_cache

    def close(self):
        self._conn.close()


def _default_socket_path() -> str:
    """Return socket path: NRFLO_SOCKET → $NRFLO_HOME/agent.sock → ~/.nrflo/agent.sock."""
    explicit = os.environ.get("NRFLO_SOCKET")
    if explicit:
        return explicit
    home = os.environ.get("NRFLO_HOME")
    if home:
        return os.path.join(home, "agent.sock")
    return os.path.join(os.path.expanduser("~"), ".nrflo", "agent.sock")


def client(
    sock_path: str = None,
    session_id: str = None,
    instance_id: str = None,
    project: str = None,
    trx: str = None,
) -> Client:
    """Create a nrflo Client, defaulting all params from spawner env vars."""
    path = sock_path or _default_socket_path()
    sid = session_id or os.environ.get("NRF_SESSION_ID", "")
    iid = instance_id or os.environ.get("NRF_WORKFLOW_INSTANCE_ID", "")
    proj = project or os.environ.get("NRFLO_PROJECT", "")
    t = trx or os.environ.get("NRF_TRX", "")
    return Client(_Connection(path), sid, iid, proj, t)


def notification() -> _Notification:
    """Return a _Notification parsed from NRFLO_NOTIFY_PAYLOAD_JSON (env-only; no socket needed)."""
    return _Notification()
