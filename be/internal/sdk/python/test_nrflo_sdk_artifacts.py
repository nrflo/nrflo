"""Unit tests for _Artifacts in nrflo_sdk.py — no running server required."""
import base64
import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nrflo_sdk

_SOCK = "/tmp/nrflo-sdk-artifacts-no-server.sock"


def _make_client():
    return nrflo_sdk.client(
        sock_path=_SOCK, session_id="s1", instance_id="i1", project="proj", trx="t1"
    )


def _make_artifacts():
    conn = nrflo_sdk._Connection(_SOCK)
    return nrflo_sdk._Artifacts(conn, "sid1", "iid1", "proj1", "trx1")


class TestArtifactsStructure(unittest.TestCase):
    def test_namespace_exists_on_client(self):
        c = _make_client()
        self.assertTrue(hasattr(c, "artifacts"))
        c.close()

    def test_is_artifacts_class(self):
        c = _make_client()
        self.assertIsInstance(c.artifacts, nrflo_sdk._Artifacts)
        c.close()

    def test_methods_are_callable(self):
        c = _make_client()
        for m in ("add", "list", "get"):
            self.assertTrue(
                callable(getattr(c.artifacts, m, None)),
                f"artifacts.{m} missing or not callable",
            )
        c.close()


class TestArtifactsAdd(unittest.TestCase):
    def test_string_encodes_utf8_base64(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"path": "/p/f.txt"}}
            arts.add("f.txt", "hello world")
        req = mock_send.call_args[0][0]
        self.assertEqual(req["method"], "artifact.add")
        params = req["params"]
        self.assertNotIn("content", params)
        self.assertIn("content_b64", params)
        self.assertEqual(base64.b64decode(params["content_b64"]), b"hello world")

    def test_string_unicode_utf8(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("f.txt", "café")
        params = mock_send.call_args[0][0]["params"]
        self.assertEqual(base64.b64decode(params["content_b64"]), "café".encode("utf-8"))

    def test_bytes_roundtrip(self):
        arts = _make_artifacts()
        raw = b"\x00\x01\x02\xff"
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("data.bin", raw)
        decoded = base64.b64decode(mock_send.call_args[0][0]["params"]["content_b64"])
        self.assertEqual(decoded, raw)

    def test_bytearray_roundtrip(self):
        arts = _make_artifacts()
        raw = bytearray(b"\xde\xad\xbe\xef")
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("data.bin", raw)
        decoded = base64.b64decode(mock_send.call_args[0][0]["params"]["content_b64"])
        self.assertEqual(decoded, bytes(raw))

    def test_no_instance_id_in_params(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("f.txt", "x")
        params = mock_send.call_args[0][0]["params"]
        self.assertIn("session_id", params)
        self.assertNotIn("instance_id", params)

    def test_oversize_raises_nrflo_error_no_send(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                arts.add("big.bin", b"x" * (32 * 1024 * 1024 + 1))
            self.assertIn("artifact too large: max 32 MiB", str(cm.exception))
            mock_send.assert_not_called()

    def test_invalid_type_raises_type_error_without_send(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(TypeError):
                arts.add("f.txt", 42)
            mock_send.assert_not_called()

    def test_server_error_propagates(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"error": {"code": -32000, "message": "write failed"}}
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                arts.add("f.txt", "x")
        self.assertEqual(cm.exception.code, -32000)
        self.assertIn("write failed", cm.exception.message)

    def test_content_type_sent_when_provided(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("f.txt", "x", content_type="text/plain")
        params = mock_send.call_args[0][0]["params"]
        self.assertEqual(params.get("content_type"), "text/plain")

    def test_content_type_absent_when_not_provided(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            arts.add("f.txt", "x")
        params = mock_send.call_args[0][0]["params"]
        self.assertNotIn("content_type", params)


class TestArtifactsList(unittest.TestCase):
    def test_returns_server_list(self):
        arts = _make_artifacts()
        items = [{"name": "a.txt", "path": "/p/a.txt"}, {"name": "b.txt", "path": "/p/b.txt"}]
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": items}
            result = arts.list()
        self.assertEqual(result, items)

    def test_empty_preserves_list_type(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": []}
            result = arts.list()
        self.assertEqual(result, [])
        self.assertIsInstance(result, list, "empty result must be list, not dict")

    def test_method_name_is_artifact_list(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": []}
            arts.list()
        self.assertEqual(mock_send.call_args[0][0]["method"], "artifact.list")

    def test_session_id_in_params(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": []}
            arts.list()
        params = mock_send.call_args[0][0]["params"]
        self.assertIn("session_id", params)
        self.assertNotIn("instance_id", params)

    def test_server_error_raises_nrflo_error(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"error": {"code": -32001, "message": "storage error"}}
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                arts.list()
        self.assertEqual(cm.exception.code, -32001)
        self.assertIn("storage error", cm.exception.message)


class TestArtifactsGet(unittest.TestCase):
    def test_returns_path_string(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"path": "/artifacts/proj/report.txt"}}
            path = arts.get("report.txt")
        self.assertEqual(path, "/artifacts/proj/report.txt")

    def test_method_name_and_name_param(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"path": "/x"}}
            arts.get("report.txt")
        req = mock_send.call_args[0][0]
        self.assertEqual(req["method"], "artifact.get")
        self.assertEqual(req["params"]["name"], "report.txt")

    def test_session_id_not_instance_id_in_params(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"path": "/x"}}
            arts.get("f.txt")
        params = mock_send.call_args[0][0]["params"]
        self.assertIn("session_id", params)
        self.assertNotIn("instance_id", params)

    def test_server_error_raises_nrflo_error(self):
        arts = _make_artifacts()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"error": {"code": -32602, "message": "artifact not found"}}
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                arts.get("missing.txt")
        self.assertEqual(cm.exception.code, -32602)
        self.assertIn("artifact not found", cm.exception.message)


if __name__ == "__main__":
    unittest.main()
