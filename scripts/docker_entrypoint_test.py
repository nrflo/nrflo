#!/usr/bin/env python3

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("docker_entrypoint.py")
SPEC = importlib.util.spec_from_file_location("docker_entrypoint", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class InitializeClaudeConfigTest(unittest.TestCase):
    def test_creates_fresh_config(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            MODULE.initialize_claude_config(home)
            self.assertEqual(
                json.loads((home / ".claude.json").read_text()),
                {"hasCompletedOnboarding": True},
            )
            self.assertEqual(
                json.loads((home / ".claude" / "settings.json").read_text()),
                {"skipDangerousModePermissionPrompt": True},
            )

    def test_preserves_existing_settings(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            path = home / ".claude.json"
            path.write_text(json.dumps({"theme": "dark", "custom": 7}))
            settings = home / ".claude" / "settings.json"
            settings.parent.mkdir()
            settings.write_text(json.dumps({"customSetting": "kept"}))
            MODULE.initialize_claude_config(home)
            self.assertEqual(
                json.loads(path.read_text()),
                {"theme": "dark", "custom": 7, "hasCompletedOnboarding": True},
            )
            self.assertEqual(
                json.loads(settings.read_text()),
                {
                    "customSetting": "kept",
                    "skipDangerousModePermissionPrompt": True,
                },
            )

    def test_leaves_completed_config_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / ".claude.json"
            settings = path.parent / ".claude" / "settings.json"
            settings.parent.mkdir()
            original = '{"hasCompletedOnboarding": true, "theme": "dark"}\n'
            original_settings = '{"skipDangerousModePermissionPrompt": true}\n'
            path.write_text(original)
            settings.write_text(original_settings)
            MODULE.initialize_claude_config(path.parent)
            self.assertEqual(path.read_text(), original)
            self.assertEqual(settings.read_text(), original_settings)


if __name__ == "__main__":
    unittest.main()
