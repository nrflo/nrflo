"""Unit tests for _Agent.consult in nrflo_sdk.py — no running server required."""
import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import nrflo_sdk

_SOCK = "/tmp/nrflo-sdk-consult-no-server.sock"


def _make_agent():
    conn = nrflo_sdk._Connection(_SOCK)
    return nrflo_sdk._Agent(conn, "sid1", "iid1", "proj1", "trx1")


class TestAgentConsultBehavior(unittest.TestCase):
    def test_returns_answer_string(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"answer": "the answer"}}
            result = agent.consult("my-consultant", "what should I do?")
        self.assertEqual(result, "the answer")

    def test_method_name_is_agent_consult(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"answer": "ok"}}
            agent.consult("consultant-x", "question?")
        req = mock_send.call_args[0][0]
        self.assertEqual(req["method"], "agent.consult")

    def test_params_contain_session_and_instance_id(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"answer": "ok"}}
            agent.consult("consultant-x", "question?")
        params = mock_send.call_args[0][0]["params"]
        self.assertEqual(params["session_id"], "sid1")
        self.assertEqual(params["instance_id"], "iid1")

    def test_params_contain_consultant_and_question(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {"answer": "ok"}}
            agent.consult("my-consultant", "what now?")
        params = mock_send.call_args[0][0]["params"]
        self.assertEqual(params["consultant"], "my-consultant")
        self.assertEqual(params["question"], "what now?")

    def test_missing_answer_key_returns_empty_string(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": {}}
            result = agent.consult("consultant-x", "question?")
        self.assertEqual(result, "")

    def test_null_result_returns_empty_string(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"result": None}
            result = agent.consult("consultant-x", "question?")
        self.assertEqual(result, "")


class TestAgentConsultValidation(unittest.TestCase):
    def test_empty_consultant_raises_value_error_before_send(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(ValueError):
                agent.consult("", "some question")
            mock_send.assert_not_called()

    def test_empty_question_raises_value_error_before_send(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(ValueError):
                agent.consult("consultant-x", "")
            mock_send.assert_not_called()

    def test_non_string_consultant_raises_value_error_before_send(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(ValueError):
                agent.consult(None, "some question")
            mock_send.assert_not_called()

    def test_non_string_question_raises_value_error_before_send(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            with self.assertRaises(ValueError):
                agent.consult("consultant-x", None)
            mock_send.assert_not_called()


class TestAgentConsultServerError(unittest.TestCase):
    def test_server_error_propagates_as_nrflo_error(self):
        agent = _make_agent()
        with unittest.mock.patch.object(nrflo_sdk._Connection, "send") as mock_send:
            mock_send.return_value = {"error": {"code": -32603, "message": "consultant unavailable"}}
            with self.assertRaises(nrflo_sdk.NrfloError) as cm:
                agent.consult("consultant-x", "some question?")
        self.assertEqual(cm.exception.code, -32603)
        self.assertIn("consultant unavailable", cm.exception.message)


if __name__ == "__main__":
    unittest.main()
