"""Synthetic contract fixtures, never real PVE acceptance evidence."""
import copy
import datetime
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import mock_open, patch
from verification import validate_machine, private_json, timestamp

ROOT = Path(__file__).resolve().parent

class VerificationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        spec = importlib.util.spec_from_file_location("lab_kit", ROOT / "lab-kit.py")
        self.kit = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.kit)
        self.time = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(minutes=2)
        self.date = self.time.isoformat()
        self.identity = "a" * 64
        self.boot = "12345678-1234-1234-1234-123456789abc"
        self.new_boot = "22345678-1234-1234-1234-123456789abc"
        self.bindings = {key: "fixture-" + key for key in self.kit.FIELDS}
        self.bindings.update(date=self.date, isolation_confirmed=True, permitted_scenarios=list(range(1,10)),
            source_manifest_sha="b"*64, connection_sha="c"*64, machine_sha256=self.identity,
            baseline_boot_id=self.boot, build_sha256="d"*64, helper_sha256="e"*64)
        self.build_artifact = self.write("build-private-copy", {"fixture":"actual local build bytes"})
        self.bindings["build_sha256"] = hashlib.sha256(self.build_artifact.read_bytes()).hexdigest()
        self.package_artifact = self.write("plan-package.tar", {"fixture":"actual exported package bytes"})
        self.bindings["plan_package_sha256"] = hashlib.sha256(self.package_artifact.read_bytes()).hexdigest()
        self.snapshot = {"id":"snapshot", "backup_id":self.bindings["source_backup_id"], "target_id":self.bindings["target_id"],
            "source_manifest_sha":"b"*64, "connection_sha":"c"*64, "captured_at":self.date,
            "target":{"details":{"capabilities":{"recovery_verify_protocol":1},"machine_identity_sha256":self.identity,"boot_id":self.boot,
                "command_results":{"machine_identity":{"state":"ok"},"boot_id":{"state":"ok"}}}},
            "storage_volumes":{"local":{"state":"ok","volumes":["local:vm-100-disk-0"]}}}
        self.snapshot_path = self.write("snapshot.json", self.snapshot)
        self.bindings["target_snapshot_sha"] = hashlib.sha256(self.snapshot_path.read_bytes()).hexdigest()
        self.attempt = {key:"" for key in ("id plan_id backup_id target_id started_at finished_at actor origin state reason source_manifest_sha snapshot_sha connection_sha baseline_identity baseline_boot_id baseline_captured_at identity_state reboot_state services_state volumes_state").split()}
        self.attempt.update(id="attempt", plan_id=self.bindings["plan_id"], backup_id=self.bindings["source_backup_id"],
            target_id=self.bindings["target_id"], started_at=self.date, finished_at=self.date, actor="operator", origin="machine-read", state="observed",
            source_manifest_sha="b"*64, snapshot_sha=self.bindings["target_snapshot_sha"], connection_sha="c"*64,
            baseline_identity=self.identity, baseline_boot_id=self.boot, baseline_captured_at=self.date,
            identity_state="passed", reboot_state="passed", services_state="passed", volumes_state="passed",
            result={"protocol":1,"observed_at":self.date,"identity":{"state":"ok","value":self.identity},"boot":{"state":"ok","value":self.new_boot},
                "services":{name:{"state":"ok","value":"active"} for name in ("pve-cluster","pvedaemon","pveproxy")},
                "storage_volumes":{"local":{"state":"ok","volumes":["local:vm-100-disk-0"]}}})
        self.plan = {"schema_version":2,"id":self.bindings["plan_id"],"backup_id":self.bindings["source_backup_id"],"target_id":self.bindings["target_id"],
            "target_snapshot_id":"snapshot", "target_snapshot_sha":self.bindings["target_snapshot_sha"],
            "source":{"fingerprint":self.bindings["source_fingerprint"]},"target":{"fingerprint":self.bindings["target_fingerprint"]},"verification_attempts":[self.attempt]}
        self.preflight = {"schema":1,"kind":"preflight","collection_authorized":True,"captured_at":self.date,"bindings":copy.deepcopy(self.bindings),
            "build_sha256":self.bindings["build_sha256"],"helper_sha256":"e"*64,"identity":{"machine_sha256":self.identity,"boot_id":self.boot}}
        self.plan_path = self.write("plan.json", self.plan)
        self.preflight_path = self.write("preflight.json", self.preflight)

    def write(self, name, data):
        p = self.root / name
        p.write_text(json.dumps(data))
        p.chmod(0o600)
        return p

    def check(self, number=8):
        self.write("plan.json", self.plan)
        self.write("preflight.json", self.preflight)
        return validate_machine(self.bindings, number, self.plan_path, self.snapshot_path, self.preflight_path)

    def test_actual_contract_narrow_observation_and_complete_dossier(self):
        result = self.check()
        self.assertEqual(result["state"], "validated_observation")
        self.assertEqual(result["volumes"], "recorded_membership_only")
        report = self.complete_steps(8)
        review = self.kit.review(report, [self.artifact,self.package_artifact], [self.plan_path,self.snapshot_path,self.preflight_path])
        self.assertEqual(review["status"], "complete")
        report["residual_failures"] = ["Dienstfunktion unbekannt"]
        self.assertEqual(self.kit.review(report, [self.artifact,self.package_artifact], [self.plan_path,self.snapshot_path,self.preflight_path])["status"], "incomplete")

    def complete_steps(self, number):
        self.artifact = self.write("operator-evidence.json", {"scope":"synthetic test fixture", "result":"operator testimony"})
        report = self.kit.scenario_report(self.bindings, number)
        for step in report["steps"]:
            step.update(outcome="pass",actor="operator",date=self.date,evidence_reference=hashlib.sha256(self.artifact.read_bytes()).hexdigest())
        return report

    def test_all_scenarios_can_complete_with_appropriate_evidence(self):
        for number in range(1,10):
            with self.subTest(number=number):
                report = self.complete_steps(number)
                artifacts = [self.artifact,self.package_artifact]
                machine = [self.plan_path,self.snapshot_path,self.preflight_path] if number in (1,2,3,8) else []
                if number == 9:
                    package = self.root / "cold.bundle"
                    package.write_bytes(b"synthetic encrypted package fixture")
                    package.chmod(0o600)
                    report["bindings"]["cold_package_sha256"] = hashlib.sha256(package.read_bytes()).hexdigest()
                    artifacts.extend([package, self.build_artifact])
                    for key in ("baseline_boot_id","plan_id","machine_sha256","target_id","helper_sha256"):
                        report["bindings"][key] = ""
                result = self.kit.review(report, artifacts, machine)
                self.assertEqual(result["status"], "complete", result)
                if number == 9:
                    self.assertEqual(result["machine_verification"]["state"], "not_applicable")
                    with self.assertRaises(ValueError):
                        self.kit.review(report, artifacts, [self.plan_path,self.snapshot_path,self.preflight_path])

    def test_scope_dates_missing_artifacts_and_human_fail_never_complete(self):
        report = self.complete_steps(9)
        for key, value in (("actor",""),("date","unknown"),("date","2099-01-01T00:00:00Z"),("evidence_reference","unknown")):
            changed = copy.deepcopy(report)
            changed["steps"][0][key] = value
            self.assertEqual(self.kit.review(changed, [self.artifact])["status"], "incomplete")
        report["steps"][0]["outcome"] = "fail"
        self.assertEqual(self.kit.review(report, [self.artifact])["status"], "failed")

    def test_latest_failed_unknown_interrupted_never_falls_back(self):
        for state in ("failed","unknown","interrupted","unsupported"):
            self.plan["verification_attempts"] = [self.attempt,dict(self.attempt,id="new",state=state)]
            with self.subTest(state=state), self.assertRaises(ValueError): self.check()

    def test_exact_bindings_and_private_baseline(self):
        for key in ("plan_id","backup_id","target_id","source_manifest_sha","snapshot_sha","connection_sha","baseline_identity","baseline_boot_id","baseline_captured_at"):
            saved = self.attempt[key]
            self.attempt[key] = "mismatch"
            with self.subTest(key=key), self.assertRaises(ValueError): self.check()
            self.attempt[key] = saved
        for key in ("machine_identity","boot_id"):
            changed = copy.deepcopy(self.snapshot)
            del changed["target"]["details"]["command_results"][key]
            self.write("snapshot.json", changed)
            digest = hashlib.sha256(self.snapshot_path.read_bytes()).hexdigest()
            self.plan["target_snapshot_sha"] = self.attempt["snapshot_sha"] = self.bindings["target_snapshot_sha"] = digest
            self.preflight["bindings"] = copy.deepcopy(self.bindings)
            with self.subTest(key=key), self.assertRaises(ValueError): self.check()

    def test_time_helper_build_and_same_boot(self):
        for obj,key,value in ((self.attempt,"finished_at","2099-01-01T00:00:00Z"), (self.attempt["result"],"observed_at","2000-01-01T00:00:00Z"),
                              (self.preflight,"build_sha256",""),(self.preflight,"helper_sha256","f"*64),(self.preflight,"collection_authorized",False)):
            saved = obj[key]; obj[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError): self.check()
            obj[key] = saved
        self.attempt["result"]["boot"]["value"] = self.boot
        self.attempt["reboot_state"] = "unchanged"
        self.assertEqual(self.check(1)["reboot"], "unchanged")
        with self.assertRaises(ValueError): self.check(8)

    def test_services_volumes_aliases_duplicates_and_private_files(self):
        for obj,key,value in ((self.attempt["result"]["services"]["pvedaemon"],"value","inactive"),
                              (self.attempt["result"]["identity"],"value","f"*64),
                              (self.attempt["result"]["storage_volumes"]["local"],"volumes",[])):
            saved=obj[key];obj[key]=value
            with self.subTest(key=key), self.assertRaises(ValueError): self.check()
            obj[key]=saved
        for obj in (self.attempt,self.attempt["result"],self.attempt["result"]["services"]["pvedaemon"]):
            obj["State"]="observed"
            with self.assertRaises(ValueError): self.check()
            del obj["State"]
        self.plan_path.write_text('{"id":"bad","id":"good"}')
        with self.assertRaises(ValueError): private_json(self.plan_path)
        self.plan_path.write_text('{"id":NaN}')
        with self.assertRaises(ValueError): private_json(self.plan_path)
        self.plan_path.chmod(0o644)
        with self.assertRaises(ValueError): private_json(self.plan_path)
        link=self.root/"link";link.symlink_to(self.snapshot_path)
        with self.assertRaises((ValueError, OSError)): private_json(link)

    def test_case_distinct_storage_ids_match_real_exact_membership_contract(self):
        self.snapshot["storage_volumes"]["Local"] = {"state":"ok", "volumes":["Local:vm-200-disk-0"]}
        self.write("snapshot.json", self.snapshot)
        digest = hashlib.sha256(self.snapshot_path.read_bytes()).hexdigest()
        self.bindings["target_snapshot_sha"] = self.plan["target_snapshot_sha"] = self.attempt["snapshot_sha"] = digest
        self.preflight["bindings"] = copy.deepcopy(self.bindings)
        self.attempt["result"]["storage_volumes"]["Local"] = {"state":"ok", "volumes":["Local:vm-200-disk-0"]}
        self.assertEqual(self.check()["volumes"], "recorded_membership_only")
        # Both keys must actually survive parsing and be checked independently.
        self.attempt["result"]["storage_volumes"]["Local"]["volumes"] = []
        with self.assertRaises(ValueError): self.check()

    def test_actual_go_rfc3339nano_timestamps_on_python39(self):
        for text in ("2026-10-09T12:34:56.123456789Z", "2026-10-09T12:34:56.1+00:00", "2026-10-09T12:34:56Z"):
            with self.subTest(text=text):
                self.assertEqual(timestamp(text).year, 2026)
        for text in ("2026-10-09", "2026-10-09T12:34:56", "2026-10-09T12:34:56.1234567890Z"):
            with self.subTest(text=text), self.assertRaises(ValueError): timestamp(text)

    def test_no_recorded_storage_is_explicitly_inapplicable(self):
        self.snapshot["storage_volumes"] = None
        self.write("snapshot.json", self.snapshot)
        digest = hashlib.sha256(self.snapshot_path.read_bytes()).hexdigest()
        self.bindings["target_snapshot_sha"] = self.plan["target_snapshot_sha"] = self.attempt["snapshot_sha"] = digest
        self.preflight["bindings"] = copy.deepcopy(self.bindings)
        self.attempt["volumes_state"] = "unknown"
        self.attempt["result"]["storage_volumes"] = {}
        self.assertEqual(self.check()["volumes"], "not_applicable_no_recorded_storage")
        self.attempt["volumes_state"] = "passed"
        with self.assertRaises(ValueError): self.check()

    def test_rejects_protocol_booleans_and_case_alias_in_inputs(self):
        self.attempt["result"]["protocol"] = True
        with self.assertRaises(ValueError): self.check()
        self.attempt["result"]["protocol"] = 1
        self.plan["ID"] = "other"
        with self.assertRaises(ValueError): self.check()
        del self.plan["ID"]
        self.preflight["Identity"] = self.preflight["identity"]
        with self.assertRaises(ValueError): self.check()

    def test_machine_id_exact_helper_definition(self):
        for raw in (b"0"*32,b"A"*32,b"a"*31,b"a"*33,b"bad"):
            with patch("builtins.open",mock_open(read_data=raw)):
                self.assertNotIn("machine_sha256",self.kit.read_identity())
        raw=b"0123456789abcdef0123456789abcdef"
        with patch("builtins.open",mock_open(read_data=b" \n"+raw+b"\n")):
            self.assertEqual(self.kit.read_identity()["machine_sha256"],hashlib.sha256(raw).hexdigest())

    def test_cli_ingests_actual_schema_and_writes_private_review(self):
        import subprocess,sys
        report=self.complete_steps(8)
        path=self.write("scenario.json",report)
        output=self.root/"validated.json"
        result=subprocess.run([sys.executable,str(ROOT/"lab-kit.py"),"review","--input",str(path),"--artifact",str(self.artifact),"--artifact",str(self.package_artifact),
            "--plan",str(self.plan_path),"--snapshot",str(self.snapshot_path),"--preflight",str(self.preflight_path),"--output",str(output)],capture_output=True,text=True)
        self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        self.assertEqual(json.loads(output.read_text())["status"],"complete")
        self.assertEqual(output.stat().st_mode & 0o777,0o600)

if __name__ == "__main__": unittest.main()
