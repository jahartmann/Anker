"""Local tooling fixtures only. Never performs a PVE acceptance run."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parent

class LabKitTests(unittest.TestCase):
    def load(self):
        spec = importlib.util.spec_from_file_location("lab_kit", ROOT / "lab-kit.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def bindings(self):
        data = json.loads((ROOT / "bindings.example.json").read_text())
        for key, value in data.items():
            if isinstance(value, str):
                data[key] = "fixture-" + key
        data["isolation_confirmed"] = True
        data["permitted_scenarios"] = [1,2,3,4,5,6,7,8,9]
        return data

    def test_missing_identity_never_becomes_a_pass(self):
        kit = self.load()
        result = kit.preflight({}, None, None, False)
        self.assertEqual(result["status"], "incomplete")
        self.assertIn("lab_id", result["missing"])
        self.assertEqual(result["machine_verification"], "not_collected")

    def test_preflight_does_not_probe_without_explicit_complete_bindings(self):
        kit = self.load()
        with patch.object(kit, "capture", side_effect=AssertionError("unexpected system query")), patch.object(kit, "read_identity", side_effect=AssertionError("unexpected identity read")):
            self.assertEqual(kit.preflight(self.bindings(), None, None, False)["observations"], {})
            self.assertEqual(kit.preflight({}, None, None, True)["observations"], {})

    def test_collects_bounded_evidence_without_configs_and_binds_binary_hashes(self):
        kit = self.load()
        with tempfile.TemporaryDirectory() as root:
            binary = Path(root) / "anker"
            binary.write_bytes(b"abc")
            with patch.object(kit, "read_identity", return_value={"machine_sha256": "machine-fixture", "boot_id": "boot-fixture"}), patch.object(kit, "capture", return_value={"state":"ok", "value":"fixture"}):
                report = kit.preflight(self.bindings(), binary, binary, True)
            self.assertEqual(report["build_sha256"], "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
            self.assertEqual(report["helper_sha256"], report["build_sha256"])
            self.assertEqual(report["status"], "incomplete")
            self.assertIn("pve_version", report["observations"])
            self.assertNotIn("config_contents", report)
            self.assertEqual(report["bindings"]["plan_id"], "fixture-plan_id")

    def test_capture_bounds_output_and_discards_errors(self):
        kit = self.load()
        import sys
        r = kit.capture([sys.executable, "-c", "print('x'*1000000)"], maximum=1024)
        self.assertEqual(r["state"], "incomplete")
        self.assertLessEqual(len(r.get("value", "")), 1024)
        r = kit.capture([sys.executable, "-c", "import sys;sys.stderr.write('secret');sys.exit(1)"])
        self.assertNotIn("secret", json.dumps(r))
        r = kit.capture([sys.executable, "-c", "import time;time.sleep(3)"], timeout=0.05)
        self.assertEqual(r["state"], "incomplete")

    def test_manual_runner_leaves_every_scenario_open_and_respects_scope(self):
        kit = self.load()
        for scenario in range(1,10):
            report = kit.scenario_report(self.bindings(), scenario)
            self.assertEqual(report["status"], "incomplete")
            self.assertTrue(report["steps"])
            self.assertTrue(any("HALT" in step["instruction"] for step in report["steps"]))
            self.assertTrue(all(step["outcome"] == "unrun" for step in report["steps"]))
            for step in report["steps"]:
                step.update(outcome="pass", actor="fixture", date="fixture", evidence_reference="fixture")
            self.assertEqual(kit.review(report)["status"], "incomplete")
            report["steps"][0]["outcome"] = "fail"
            self.assertEqual(kit.review(report)["status"], "failed")
        bindings = self.bindings()
        bindings["permitted_scenarios"] = [1]
        with self.assertRaises(ValueError):
            kit.scenario_report(bindings, 9)

    def test_cli_creates_incomplete_private_reports_without_host_access(self):
        import subprocess
        import sys
        with tempfile.TemporaryDirectory() as root:
            bindings = Path(root) / "bindings.json"
            bindings.write_text(json.dumps(self.bindings()))
            bindings.chmod(0o600)
            report = Path(root) / "scenario.json"
            result = subprocess.run([sys.executable, str(ROOT / "lab-kit.py"), "scenario", "--bindings", str(bindings), "--number", "8", "--output", str(report)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertEqual(json.loads(report.read_text())["status"], "incomplete")
            result = subprocess.run([sys.executable, str(ROOT / "lab-kit.py"), "review", "--input", str(report)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn("missing", result.stdout)
            output = Path(root) / "preflight.json"
            result = subprocess.run([sys.executable, str(ROOT / "lab-kit.py"), "preflight", "--bindings", str(bindings), "--output", str(output)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertEqual(json.loads(output.read_text())["observations"], {})

    def test_json_inputs_require_private_regular_bounded_files(self):
        kit = self.load()
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "bindings.json"
            path.write_text('{"lab_id":"fixture"}')
            path.chmod(0o600)
            self.assertEqual(kit.read_json(path), {"lab_id":"fixture"})
            path.chmod(0o644)
            with self.assertRaises(ValueError): kit.read_json(path)
            path.chmod(0o600)
            link = Path(root) / "link.json"
            link.symlink_to(path)
            with self.assertRaises((OSError, ValueError)): kit.read_json(link)
            path.write_bytes(b" " * ((1 << 20) + 1))
            with self.assertRaises(ValueError): kit.read_json(path)
            path.write_text('{"id":"first","id":"second"}')
            with self.assertRaises(ValueError): kit.read_json(path)
            path.write_text('{"id":NaN}')
            with self.assertRaises(ValueError): kit.read_json(path)
            path.write_text('{"lab_id":"fixture"}')
            Path(root).chmod(0o755)
            try:
                with self.assertRaises(ValueError): kit.read_json(path)
            finally:
                Path(root).chmod(0o700)

    def test_cli_fifo_input_is_rejected_promptly_without_reader_hang(self):
        import os
        import subprocess
        import sys
        with tempfile.TemporaryDirectory() as root:
            fifo = Path(root) / "scenario.json"
            os.mkfifo(fifo, 0o600)
            # subprocess.run kills and waits for the child on timeout, so a RED
            # regression cannot hang the suite or leave a blocked child behind.
            for args in (["review", "--input", str(fifo)],
                         ["scenario", "--bindings", str(fifo), "--number", "8", "--output", str(Path(root) / "output.json")]):
                result = subprocess.run([sys.executable, str(ROOT / "lab-kit.py"), *args],
                                        capture_output=True, text=True, timeout=1)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("Laborprotokoll unvollständig", result.stdout)

    def test_reports_are_private_no_overwrite_and_no_symlink_write(self):
        kit = self.load()
        with tempfile.TemporaryDirectory() as root:
            target = Path(root) / "report.json"
            kit.save_private(target, {"status":"incomplete"})
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                kit.save_private(target, {})
            link = Path(root) / "link"
            link.symlink_to(target)
            with self.assertRaises(FileExistsError):
                kit.save_private(link, {})

if __name__ == "__main__":
    unittest.main()
