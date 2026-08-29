from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
PLUGIN_CREATOR = (
    REPO / "apps" / "desktop" / "resources" / "system-skills" / "plugin-creator"
)
SCRIPTS = PLUGIN_CREATOR / "scripts"


class PluginCreatorScriptTests(unittest.TestCase):
    def run_script(self, script: str, *arguments: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, "-B", str(SCRIPTS / script), *arguments],
            capture_output=True,
            text=True,
            check=False,
        )

    def test_scaffold_and_preflight_a_multi_skill_plugin(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "authoring"
            created = self.run_script(
                "init_plugin.py",
                "operations-suite",
                "--path",
                str(parent),
                "--description",
                "Reusable operations workflows",
                "--skill",
                "summarize=Summarize operational records.",
                "--skill",
                "follow-up=Prepare follow-up actions.",
                "--with-mcp",
            )

            self.assertEqual(0, created.returncode, created.stderr)
            creation = json.loads(created.stdout)
            self.assertEqual(
                {
                    "follow-up",
                    "summarize",
                },
                set(creation["skills"]),
            )

            plugin_root = parent / "operations-suite"
            self.assertTrue((plugin_root / "plugin.json").is_file())
            self.assertTrue((plugin_root / "skills" / "summarize" / "SKILL.md").is_file())
            self.assertTrue((plugin_root / "skills" / "follow-up" / "SKILL.md").is_file())
            self.assertTrue((plugin_root / "mcp.json").is_file())

            validated = self.run_script("validate_plugin.py", str(plugin_root))
            self.assertEqual(0, validated.returncode, validated.stderr)
            validation = json.loads(validated.stdout)
            self.assertEqual(
                {
                    "valid": True,
                    "skill_count": 2,
                    "server_count": 0,
                    "errors": [],
                },
                {
                    key: validation[key]
                    for key in ("valid", "skill_count", "server_count", "errors")
                },
            )

    def test_scaffold_refuses_to_overwrite_an_existing_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            arguments = (
                "stable-plugin",
                "--path",
                str(parent),
                "--description",
                "Stable candidate",
            )
            first = self.run_script("init_plugin.py", *arguments)
            self.assertEqual(0, first.returncode, first.stderr)
            manifest = parent / "stable-plugin" / "plugin.json"
            original = manifest.read_bytes()

            second = self.run_script("init_plugin.py", *arguments)
            self.assertNotEqual(0, second.returncode)
            self.assertEqual(original, manifest.read_bytes())
            self.assertIn("target already exists", json.loads(second.stdout)["error"])

    def test_scaffold_rejects_invalid_input_before_writing_a_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            result = self.run_script(
                "init_plugin.py",
                "Invalid Plugin",
                "--path",
                str(parent),
                "--description",
                "Invalid candidate",
            )

            self.assertNotEqual(0, result.returncode)
            self.assertEqual([], list(parent.iterdir()))
            self.assertIn("naming rules", json.loads(result.stdout)["error"])

    def test_preflight_reports_a_missing_routed_resource(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            created = self.run_script(
                "init_plugin.py",
                "resource-plugin",
                "--path",
                str(parent),
                "--description",
                "Resource validation",
                "--skill",
                "audit=Audit a package.",
            )
            self.assertEqual(0, created.returncode, created.stderr)
            skill = parent / "resource-plugin" / "skills" / "audit" / "SKILL.md"
            skill.write_text(
                skill.read_text(encoding="utf-8")
                + "\nRead references/missing.md before continuing.\n",
                encoding="utf-8",
            )

            validated = self.run_script(
                "validate_plugin.py",
                str(parent / "resource-plugin"),
            )
            self.assertNotEqual(0, validated.returncode)
            validation = json.loads(validated.stdout)
            self.assertFalse(validation["valid"])
            self.assertTrue(
                any("references/missing.md" in error for error in validation["errors"])
            )

if __name__ == "__main__":
    unittest.main()
