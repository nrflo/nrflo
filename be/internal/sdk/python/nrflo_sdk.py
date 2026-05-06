"""nrflo Python SDK — persistent Unix-socket client for script-mode agents.

Auto-populates session_id, instance_id, project, and trx from env vars set by
the spawner (NRF_SESSION_ID, NRF_WORKFLOW_INSTANCE_ID, NRFLO_PROJECT, NRF_TRX).
Socket path defaults to /tmp/nrflo/nrflo.sock (override: NRFLO_SOCK_PATH).

Usage:
    import sys
    sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
    import nrflo_sdk
    c = nrflo_sdk.client()
    c.findings.add("result", "done")
    c.agent.finished()
"""

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


class Client:
    """nrflo script-mode client. Obtain via nrflo_sdk.client()."""

    def __init__(self, conn: _Connection, sid: str, iid: str, proj: str, trx: str):
        self._conn = conn
        self._sid = sid
        self._iid = iid
        self._proj = proj
        self._trx = trx
        self._ctx_cache = None
        self.findings = _Findings(conn, sid, iid, proj, trx)
        self.project_findings = _ProjectFindings(conn, sid, iid, proj, trx)
        self.agent = _Agent(conn, sid, iid, proj, trx)

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

    def close(self):
        self._conn.close()


def client(
    sock_path: str = None,
    session_id: str = None,
    instance_id: str = None,
    project: str = None,
    trx: str = None,
) -> Client:
    """Create a nrflo Client, defaulting all params from spawner env vars."""
    path = sock_path or os.environ.get("NRFLO_SOCK_PATH", "/tmp/nrflo/nrflo.sock")
    sid = session_id or os.environ.get("NRF_SESSION_ID", "")
    iid = instance_id or os.environ.get("NRF_WORKFLOW_INSTANCE_ID", "")
    proj = project or os.environ.get("NRFLO_PROJECT", "")
    t = trx or os.environ.get("NRF_TRX", "")
    return Client(_Connection(path), sid, iid, proj, t)
