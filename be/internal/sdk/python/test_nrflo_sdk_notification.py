"""Unit tests for _Notification in nrflo_sdk.py — no running server required."""
import json
import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nrflo_sdk

_SOCK = "/tmp/nrflo-sdk-notification-no-server.sock"

_FIXTURE = {
    "event_type": "workflow_completed",
    "project_id": "proj-abc",
    "project_name": "My Project",
    "workflow": "feature",
    "instance_id": "inst-42",
    "ticket_id": "TKT-7",
    "ticket_name": "Add login page",
    "agent_type": "qa-verifier",
    "reason": "all tests passed",
    "workflow_final_result": "Implementation complete and verified",
}


def _make_client():
    return nrflo_sdk.client(
        sock_path=_SOCK, session_id="s1", instance_id="i1", project="proj", trx="t1"
    )


class TestNotificationProperties(unittest.TestCase):
    def test_all_string_properties_match_fixture(self):
        """Populated env → all string properties reflect fixture values."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            n = nrflo_sdk._Notification()
            self.assertEqual(n.event_type, _FIXTURE["event_type"])
            self.assertEqual(n.project_id, _FIXTURE["project_id"])
            self.assertEqual(n.project_name, _FIXTURE["project_name"])
            self.assertEqual(n.workflow, _FIXTURE["workflow"])
            self.assertEqual(n.instance_id, _FIXTURE["instance_id"])
            self.assertEqual(n.ticket_id, _FIXTURE["ticket_id"])
            self.assertEqual(n.ticket_name, _FIXTURE["ticket_name"])
            self.assertEqual(n.agent_type, _FIXTURE["agent_type"])
            self.assertEqual(n.reason, _FIXTURE["reason"])

    def test_summary_reads_workflow_final_result(self):
        """summary property pulls from workflow_final_result key, not summary."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            n = nrflo_sdk._Notification()
            self.assertEqual(n.summary, _FIXTURE["workflow_final_result"])

    def test_raw_returns_full_parsed_dict(self):
        """raw property returns the full parsed payload dict."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            n = nrflo_sdk._Notification()
            self.assertEqual(n.raw, _FIXTURE)

    def test_absent_keys_return_empty_string(self):
        """Properties return '' when keys are absent from payload."""
        payload = {"event_type": "test_event"}
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(payload)},
            clear=False,
        ):
            n = nrflo_sdk._Notification()
            self.assertEqual(n.project_id, "")
            self.assertEqual(n.project_name, "")
            self.assertEqual(n.workflow, "")
            self.assertEqual(n.instance_id, "")
            self.assertEqual(n.ticket_id, "")
            self.assertEqual(n.ticket_name, "")
            self.assertEqual(n.agent_type, "")
            self.assertEqual(n.reason, "")
            self.assertEqual(n.summary, "")


class TestNotificationErrors(unittest.TestCase):
    def test_missing_env_raises_nrflo_error(self):
        """Absent NRFLO_NOTIFY_PAYLOAD_JSON raises NrfloError(0, exact message)."""
        with unittest.mock.patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                nrflo_sdk._Notification()
            self.assertEqual(cm.exception.code, 0)
            self.assertIn("no notification payload in env", cm.exception.message)
            self.assertIn("NRFLO_NOTIFY_PAYLOAD_JSON", cm.exception.message)

    def test_empty_string_env_raises_nrflo_error(self):
        """Empty-string NRFLO_NOTIFY_PAYLOAD_JSON raises the same NrfloError."""
        with unittest.mock.patch.dict(
            os.environ, {"NRFLO_NOTIFY_PAYLOAD_JSON": ""}, clear=False
        ):
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                nrflo_sdk._Notification()
            self.assertEqual(cm.exception.code, 0)
            self.assertIn("no notification payload in env", cm.exception.message)

    def test_invalid_json_raises_nrflo_error(self):
        """Malformed JSON in env raises NrfloError indicating invalid JSON."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": "{not-valid-json"},
            clear=False,
        ):
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                nrflo_sdk._Notification()
            self.assertEqual(cm.exception.code, 0)
            self.assertIn("invalid JSON", cm.exception.message)


class TestModuleLevelNotification(unittest.TestCase):
    def test_returns_notification_instance_without_client(self):
        """nrflo_sdk.notification() works without instantiating a Client."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            n = nrflo_sdk.notification()
            self.assertIsInstance(n, nrflo_sdk._Notification)

    def test_module_level_properties_match_fixture(self):
        """Module-level notification() parses properties correctly."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            n = nrflo_sdk.notification()
            self.assertEqual(n.event_type, "workflow_completed")
            self.assertEqual(n.summary, _FIXTURE["workflow_final_result"])

    def test_module_level_missing_env_raises_nrflo_error(self):
        """Module-level notification() propagates NrfloError when env absent."""
        with unittest.mock.patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(nrflo_sdk.NrfloError):
                nrflo_sdk.notification()


class TestClientNotificationCache(unittest.TestCase):
    def test_cache_identity_same_instance_across_calls(self):
        """Client.notification() returns the same instance on repeated calls."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            c = _make_client()
            n1 = c.notification()
            n2 = c.notification()
            self.assertIs(n1, n2)
            c.close()

    def test_client_notification_is_notification_class(self):
        """Client.notification() returns a _Notification instance."""
        with unittest.mock.patch.dict(
            os.environ,
            {"NRFLO_NOTIFY_PAYLOAD_JSON": json.dumps(_FIXTURE)},
            clear=False,
        ):
            c = _make_client()
            n = c.notification()
            self.assertIsInstance(n, nrflo_sdk._Notification)
            c.close()

    def test_notification_cache_starts_as_none(self):
        """_notification_cache is None before the first notification() call."""
        c = _make_client()
        self.assertIsNone(c._notification_cache)
        c.close()

    def test_notification_method_is_callable(self):
        """Client.notification is callable."""
        c = _make_client()
        self.assertTrue(callable(getattr(c, "notification", None)))
        c.close()


if __name__ == "__main__":
    unittest.main()
