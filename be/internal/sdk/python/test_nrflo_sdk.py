"""Unit tests for nrflo_sdk.py — pure stdlib, no running server required."""
import os
import sys
import unittest

# Import from current directory (the Go test copies the SDK here).
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nrflo_sdk


class TestNrfloError(unittest.TestCase):
    def test_attributes(self):
        err = nrflo_sdk.NrfloError(404, "not found")
        self.assertEqual(err.code, 404)
        self.assertEqual(err.message, "not found")

    def test_str_contains_code_and_message(self):
        err = nrflo_sdk.NrfloError(-32604, "session missing")
        self.assertIn("-32604", str(err))
        self.assertIn("session missing", str(err))

    def test_is_exception(self):
        err = nrflo_sdk.NrfloError(0, "x")
        self.assertIsInstance(err, Exception)


class TestCheck(unittest.TestCase):
    def test_error_response_raises_nrflo_error(self):
        resp = {"error": {"code": -32604, "message": "not found"}}
        with self.assertRaises(nrflo_sdk.NrfloError) as cm:
            nrflo_sdk._check(resp)
        self.assertEqual(cm.exception.code, -32604)
        self.assertIn("not found", cm.exception.message)

    def test_success_response_returns_result(self):
        resp = {"result": {"key": "val"}}
        self.assertEqual(nrflo_sdk._check(resp), {"key": "val"})

    def test_missing_result_returns_empty_dict(self):
        resp = {}
        self.assertEqual(nrflo_sdk._check(resp), {})

    def test_null_result_returns_empty_dict(self):
        resp = {"result": None}
        self.assertEqual(nrflo_sdk._check(resp), {})


class TestConnectionNoServer(unittest.TestCase):
    _SOCK = "/tmp/nrflo-sdk-unit-test-no-server.sock"

    def test_send_raises_nrflo_error_when_socket_absent(self):
        conn = nrflo_sdk._Connection(self._SOCK)
        with self.assertRaises(nrflo_sdk.NrfloError):
            conn.send({"method": "test"})

    def test_close_is_idempotent(self):
        conn = nrflo_sdk._Connection(self._SOCK)
        conn.close()
        conn.close()  # Must not raise

    def test_close_after_failed_send(self):
        conn = nrflo_sdk._Connection(self._SOCK)
        try:
            conn.send({"method": "test"})
        except nrflo_sdk.NrfloError:
            pass
        conn.close()  # Must not raise


class TestClientStructure(unittest.TestCase):
    _SOCK = "/tmp/nrflo-sdk-unit-test-no-server.sock"

    def _make_client(self):
        return nrflo_sdk.client(
            sock_path=self._SOCK,
            session_id="s",
            instance_id="i",
            project="p",
            trx="t",
        )

    def test_findings_namespace_exists(self):
        c = self._make_client()
        self.assertTrue(hasattr(c, "findings"))
        c.close()

    def test_findings_has_expected_methods(self):
        c = self._make_client()
        for method in ("add", "add_bulk", "get", "append", "append_bulk", "delete"):
            self.assertTrue(callable(getattr(c.findings, method, None)),
                            f"findings.{method} missing or not callable")
        c.close()

    def test_project_findings_namespace_exists(self):
        c = self._make_client()
        self.assertTrue(hasattr(c, "project_findings"))
        for method in ("add", "add_bulk", "get", "append", "append_bulk", "delete"):
            self.assertTrue(callable(getattr(c.project_findings, method, None)),
                            f"project_findings.{method} missing or not callable")
        c.close()

    def test_agent_namespace_exists(self):
        c = self._make_client()
        self.assertTrue(hasattr(c, "agent"))
        for method in ("finished", "fail", "continue_", "callback"):
            self.assertTrue(callable(getattr(c.agent, method, None)),
                            f"agent.{method} missing or not callable")
        c.close()

    def test_top_level_methods_exist(self):
        c = self._make_client()
        for method in ("context", "user_instructions", "callback_info",
                       "previous_data", "skip", "log", "close"):
            self.assertTrue(callable(getattr(c, method, None)),
                            f"Client.{method} missing or not callable")
        c.close()

    def test_context_fails_without_server(self):
        c = self._make_client()
        with self.assertRaises(nrflo_sdk.NrfloError):
            c.context()
        c.close()

    def test_context_cache_not_populated_on_error(self):
        c = self._make_client()
        try:
            c.context()
        except nrflo_sdk.NrfloError:
            pass
        # _ctx_cache must remain None after a failed call
        self.assertIsNone(c._ctx_cache)
        c.close()

    def test_client_is_client_class(self):
        c = self._make_client()
        self.assertIsInstance(c, nrflo_sdk.Client)
        c.close()


class TestFindingsGetValidation(unittest.TestCase):
    """Client-side validation in _Findings.get() — no running server needed."""

    _SOCK = "/tmp/nrflo-sdk-unit-test-no-server.sock"

    def _make_client(self):
        return nrflo_sdk.client(
            sock_path=self._SOCK,
            session_id="s",
            instance_id="i",
            project="p",
            trx="t",
        )

    def test_agent_type_and_layer_raises_value_error(self):
        """Both agent_type and layer raises ValueError before the socket is touched."""
        c = self._make_client()
        with self.assertRaises(ValueError) as cm:
            c.findings.get(agent_type="implementor", layer=1)
        self.assertIn("mutually exclusive", str(cm.exception).lower())
        c.close()

    def test_layer_zero_raises_nrflo_error_not_value_error(self):
        """layer=0 is valid (0 is not None); error is NrfloError (no server), not ValueError."""
        c = self._make_client()
        try:
            c.findings.get(layer=0)
            self.fail("expected NrfloError (no server), got no exception")
        except nrflo_sdk.NrfloError:
            pass  # expected: socket missing, but validation passed
        except ValueError as e:
            self.fail(f"ValueError raised for layer=0 (0 is a valid layer): {e}")
        finally:
            c.close()

    def test_agent_type_alone_no_value_error(self):
        """Setting only agent_type (no layer) does not raise ValueError."""
        c = self._make_client()
        try:
            c.findings.get(agent_type="setup-analyzer")
        except nrflo_sdk.NrfloError:
            pass  # expected: no server
        except ValueError as e:
            self.fail(f"ValueError raised but layer is None: {e}")
        finally:
            c.close()

    def test_layer_alone_no_value_error(self):
        """Setting only layer (no agent_type) does not raise ValueError."""
        c = self._make_client()
        try:
            c.findings.get(layer=3)
        except nrflo_sdk.NrfloError:
            pass  # expected: no server
        except ValueError as e:
            self.fail(f"ValueError raised but agent_type is None: {e}")
        finally:
            c.close()


class TestClientLog(unittest.TestCase):
    """Structural and validation tests for Client.log() — no running server needed."""

    _SOCK = "/tmp/nrflo-sdk-unit-test-no-server.sock"

    def _make_client(self):
        return nrflo_sdk.client(
            sock_path=self._SOCK,
            session_id="s",
            instance_id="i",
            project="p",
            trx="t",
        )

    def test_log_method_callable(self):
        c = self._make_client()
        self.assertTrue(callable(getattr(c, "log", None)), "Client.log must be callable")
        c.close()

    def test_log_raises_nrflo_error_without_server(self):
        """log() with no server raises NrfloError (socket absent), not TypeError."""
        c = self._make_client()
        try:
            c.log("text", "hello")
            self.fail("expected NrfloError (no server)")
        except nrflo_sdk.NrfloError:
            pass  # expected
        except TypeError as e:
            self.fail(f"TypeError raised — signature mismatch: {e}")
        finally:
            c.close()

    def test_log_with_payload_raises_nrflo_error_not_type_error(self):
        """log(type, message, payload=...) signature is valid; error comes from socket."""
        c = self._make_client()
        try:
            c.log("tool", "ran bash", payload={"k": "v"})
            self.fail("expected NrfloError (no server)")
        except nrflo_sdk.NrfloError:
            pass
        except TypeError as e:
            self.fail(f"TypeError on log with payload — signature broken: {e}")
        finally:
            c.close()

    def test_log_default_type_text_no_type_error(self):
        """Calling log with only message (type defaults to 'text') has correct signature."""
        c = self._make_client()
        try:
            c.log(message="just a message")
            self.fail("expected NrfloError (no server)")
        except nrflo_sdk.NrfloError:
            pass
        except TypeError as e:
            self.fail(f"TypeError for log(message=...) — signature wrong: {e}")
        finally:
            c.close()


if __name__ == "__main__":
    unittest.main()
