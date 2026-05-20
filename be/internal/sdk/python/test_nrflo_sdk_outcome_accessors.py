"""Tests for the 4 outcome accessor methods added to the Python SDK client."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nrflo_sdk

_SOCK = "/tmp/nrflo-sdk-unit-test-no-server.sock"


def _make_client():
    return nrflo_sdk.client(
        sock_path=_SOCK,
        session_id="s",
        instance_id="i",
        project="p",
        trx="t",
    )


class TestOutcomeAccessors(unittest.TestCase):
    """workflow_result, workflow_status, workflow_final_result, failure_reason."""

    def test_outcome_accessors_with_cached_context(self):
        """All 4 outcome accessors read from _ctx_cache without touching the socket."""
        c = _make_client()
        c._ctx_cache = {
            "workflow_result": "pass",
            "workflow_status": "completed",
            "workflow_final_result": "great outcome",
            "failure_reason": "something went wrong",
        }
        self.assertEqual(c.workflow_result(), "pass")
        self.assertEqual(c.workflow_status(), "completed")
        self.assertEqual(c.workflow_final_result(), "great outcome")
        self.assertEqual(c.failure_reason(), "something went wrong")
        c.close()

    def test_outcome_accessors_fail_result(self):
        """fail result and failed status are correctly read from cache."""
        c = _make_client()
        c._ctx_cache = {
            "workflow_result": "fail",
            "workflow_status": "failed",
            "workflow_final_result": "",
            "failure_reason": "agent exceeded max retries",
        }
        self.assertEqual(c.workflow_result(), "fail")
        self.assertEqual(c.workflow_status(), "failed")
        self.assertEqual(c.workflow_final_result(), "")
        self.assertEqual(c.failure_reason(), "agent exceeded max retries")
        c.close()

    def test_outcome_accessors_default_empty_string_when_key_absent(self):
        """When context dict lacks outcome keys, each accessor returns ''."""
        c = _make_client()
        c._ctx_cache = {}
        self.assertEqual(c.workflow_result(), "")
        self.assertEqual(c.workflow_status(), "")
        self.assertEqual(c.workflow_final_result(), "")
        self.assertEqual(c.failure_reason(), "")
        c.close()

    def test_outcome_accessors_active_instance(self):
        """Active WFI (no result yet) yields empty workflow_result."""
        c = _make_client()
        c._ctx_cache = {
            "workflow_result": "",
            "workflow_status": "active",
            "workflow_final_result": "",
            "failure_reason": "",
        }
        self.assertEqual(c.workflow_result(), "")
        self.assertEqual(c.workflow_status(), "active")
        c.close()


if __name__ == "__main__":
    unittest.main()
