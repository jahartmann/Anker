"""Offline validation of actual Anker Plan JSON, never an SSH runner or attestation."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import stat

SHA = re.compile(r"[0-9a-f]{64}\Z")
BOOT = re.compile(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\Z")
SERVICES = {"pve-cluster", "pvedaemon", "pveproxy"}
ATTEMPT_FIELDS = set(("id plan_id backup_id target_id started_at finished_at actor origin state reason "
    "source_manifest_sha snapshot_sha connection_sha baseline_identity baseline_boot_id baseline_captured_at "
    "identity_state reboot_state services_state volumes_state result").split())


def reject_constant(value):
    raise ValueError("Ungültige JSON-Zahl")


def object_pairs(pairs):
    obj = {}
    folded = set()
    for key, value in pairs:
        if key.casefold() in folded:
            raise ValueError("Doppelter JSON-Schlüssel")
        folded.add(key.casefold())
        obj[key] = value
    return obj


def exact(obj, required, optional=()):
    if not isinstance(obj, dict) or not set(required) <= set(obj) or set(obj) - set(required) - set(optional):
        raise ValueError("Unbekannte oder fehlende Protokollfelder")
    if any(v is None for v in obj.values()):
        raise ValueError("Null ist kein Nachweis")
    return obj


def timestamp(value):
    if not isinstance(value, str):
        raise ValueError("Prüfzeit fehlt")
    match = re.fullmatch(r"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})", value)
    if not match:
        raise ValueError("RFC3339-Prüfzeit erforderlich")
    # Go emits RFC3339Nano; Python 3.9 accepts only 3/6 fractional digits.
    # Microsecond truncation affects clock-window comparisons only; raw baseline
    # strings and private snapshot hashes are still compared exactly.
    fraction = "." + match[2][:6].ljust(6, "0") if match[2] else ""
    normalized = match[1] + fraction + match[3].replace("Z", "+00:00")
    try:
        result = datetime.datetime.fromisoformat(normalized)
    except ValueError:
        raise ValueError("Prüfzeit ungültig") from None
    if result.tzinfo is None:
        raise ValueError("Zeitzone fehlt")
    return result


def private_bytes(path, maximum=1 << 20):
    p = Path(path)
    if p.parent.stat().st_mode & 0o077:
        raise ValueError("Beleg-Elternverzeichnis muss privat sein")
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or info.st_size > maximum:
            raise ValueError("Beleg muss private gewöhnliche Datei innerhalb Größenlimit sein")
        raw = stream.read(maximum + 1)
    if len(raw) > maximum:
        raise ValueError("Beleg überschreitet Größenlimit")
    return raw


def private_json(path, maximum=1 << 20):
    raw = private_bytes(path, maximum)
    obj = json.loads(raw, object_pairs_hook=object_pairs, parse_constant=reject_constant)
    if not isinstance(obj, dict):
        raise ValueError("JSON-Objekt erforderlich")
    return obj, hashlib.sha256(raw).hexdigest()


def validate_machine(bindings, number, plan_path, snapshot_path, preflight_path):
    """Latest stored attempt only; CLI success alone is never evidence success."""
    plan, plan_digest = private_json(plan_path, 32 << 20)
    snapshot, snapshot_digest = private_json(snapshot_path, 32 << 20)
    preflight, preflight_digest = private_json(preflight_path)
    if type(plan.get("schema_version")) is not int or plan["schema_version"] != 2 or "storage_volumes" not in snapshot:
        raise ValueError("Plan-/Aufnahmeschema fehlt")
    for key in ("source_manifest_sha", "target_snapshot_sha", "connection_sha", "machine_sha256", "build_sha256", "helper_sha256"):
        if not SHA.fullmatch(bindings.get(key, "")):
            raise ValueError("Gebundener Digest fehlt: " + key)
    if snapshot_digest != bindings["target_snapshot_sha"] or plan.get("target_snapshot_sha") != snapshot_digest:
        raise ValueError("Private Zielaufnahme stimmt nicht bytegenau überein")
    for field, binding in (("id", "plan_id"), ("backup_id", "source_backup_id"), ("target_id", "target_id")):
        if plan.get(field) != bindings[binding]:
            raise ValueError("Planbindung abweichend")
    if snapshot.get("id") != plan.get("target_snapshot_id") or snapshot.get("backup_id") != plan["backup_id"] or snapshot.get("target_id") != plan["target_id"]:
        raise ValueError("Aufnahmebindung abweichend")
    for field in ("source", "target"):
        if plan.get(field, {}).get("fingerprint") != bindings[field + "_fingerprint"]:
            raise ValueError("Inventarbindung abweichend")
    if preflight.get("kind") != "preflight" or (type(preflight.get("schema")) is not int or preflight["schema"] != 1) or preflight.get("bindings") != bindings or preflight.get("collection_authorized") is not True:
        raise ValueError("Lokaler Vorbeleg fehlt oder gehört zu anderem Lauf")
    for key in ("build_sha256", "helper_sha256"):
        if preflight.get(key) != bindings[key]:
            raise ValueError("Separat gelesene Build-/Helferbytes abweichend")
    if preflight.get("identity", {}).get("machine_sha256") != bindings["machine_sha256"]:
        raise ValueError("Lokale Maschinenidentität abweichend")
    attempts = plan.get("verification_attempts")
    if not isinstance(attempts, list) or not attempts or len(attempts) > 100:
        raise ValueError("Maschinenversuch fehlt")
    seen = set()
    previous = None
    for attempt in attempts:
        exact(attempt, ATTEMPT_FIELDS - {"result"}, {"result"})
        if not isinstance(attempt["id"], str) or not attempt["id"] or attempt["id"] in seen:
            raise ValueError("Versuchsidentität ungültig")
        for key, expected in (("plan_id", plan["id"]), ("backup_id", plan["backup_id"]), ("target_id", plan["target_id"]), ("origin", "machine-read")):
            if attempt[key] != expected:
                raise ValueError("Fremder Versuch im Planverlauf")
        if attempt["state"] not in ("unknown", "unsupported", "interrupted", "failed", "observed"):
            raise ValueError("Unbekannter Versuchszustand")
        seen.add(attempt["id"])
        started = timestamp(attempt["started_at"])
        if previous is not None and started < previous:
            raise ValueError("Versuchsreihenfolge ungültig")
        previous = started
    a = attempts[-1]
    result = exact(a.get("result"), ("protocol", "observed_at", "identity", "boot", "services", "storage_volumes"))
    if a["origin"] != "machine-read" or a["state"] != "observed" or not a["actor"] or (type(result["protocol"]) is not int or result["protocol"] != 1):
        raise ValueError("Neuester Versuch ist kein abgeschlossener lesender Beleg")
    for field, value in (("plan_id", plan["id"]), ("backup_id", plan["backup_id"]), ("target_id", plan["target_id"]),
                         ("source_manifest_sha", bindings["source_manifest_sha"]), ("snapshot_sha", snapshot_digest),
                         ("connection_sha", bindings["connection_sha"])):
        if a[field] != value:
            raise ValueError("Versuchsbindung abweichend")
    for field in ("source_manifest_sha", "connection_sha"):
        if snapshot.get(field) != bindings[field]:
            raise ValueError("Quell-/Vertrauensbindung abweichend")
    details = snapshot.get("target", {}).get("details", {})
    capability = details.get("capabilities", {}).get("recovery_verify_protocol")
    if type(capability) is not int or capability != 1:
        raise ValueError("Aufgenommene Helferfähigkeit fehlt")
    commands = details.get("command_results", {})
    if commands.get("machine_identity", {}).get("state") != "ok" or commands.get("boot_id", {}).get("state") != "ok":
        raise ValueError("Private positive Vorherabfrage fehlt; Preflight kann sie nicht ersetzen")
    identity, boot = details.get("machine_identity_sha256", ""), details.get("boot_id", "")
    if identity != bindings["machine_sha256"] or not BOOT.fullmatch(boot) or boot != bindings["baseline_boot_id"]:
        raise ValueError("Private Baseline abweichend")
    if (a["baseline_identity"], a["baseline_boot_id"], a["baseline_captured_at"]) != (identity, boot, snapshot.get("captured_at")):
        raise ValueError("Versuch hat andere private Baseline")
    start, end, observed = map(timestamp, (a["started_at"], a["finished_at"], result["observed_at"]))
    tolerance = datetime.timedelta(minutes=5)
    lab_start = timestamp(bindings["date"])
    if not lab_start <= start <= end or not start-tolerance <= observed <= end+tolerance or end > datetime.datetime.now(datetime.timezone.utc)+tolerance:
        raise ValueError("Prüfzeit außerhalb gebundenem Laborlauf")
    if not lab_start-tolerance <= timestamp(preflight.get("captured_at")) <= end+tolerance or timestamp(a["baseline_captured_at"]) > start:
        raise ValueError("Vorbeleg-/Baselinezeit ungültig")
    for field in ("identity", "boot"):
        exact(result[field], ("state", "value"))
        if result[field]["state"] != "ok":
            raise ValueError("Aktueller Identitäts-/Bootnachweis unbekannt")
    if result["identity"]["value"] != identity or a["identity_state"] != "passed" or not BOOT.fullmatch(result["boot"]["value"]):
        raise ValueError("Aktuelle Maschine abweichend")
    reboot = "unchanged" if result["boot"]["value"] == boot else "passed"
    if a["reboot_state"] != reboot or (number == 8 and reboot != "passed"):
        raise ValueError("Neustart nicht belegt")
    services = exact(result["services"], SERVICES)
    for query in services.values():
        exact(query, ("state", "value"))
        if query != {"state": "ok", "value": "active"}:
            raise ValueError("Dienstzustand negativ oder unbekannt")
    if a["services_state"] != "passed":
        raise ValueError("Dienstbeleg unbekannt")
    baseline_volumes = snapshot.get("storage_volumes") or {}
    volumes = exact(result["storage_volumes"], baseline_volumes)
    for key, current in volumes.items():
        exact(current, ("state", "volumes"), ("error",))
        prior = baseline_volumes[key]
        if prior.get("state") != "ok" or current["state"] != "ok" or not isinstance(current["volumes"], list) or not isinstance(prior.get("volumes"), list):
            raise ValueError("Volumeabfrage unbekannt")
        if any(not isinstance(v, str) for v in current["volumes"] + prior["volumes"]) or not set(prior["volumes"]) <= set(current["volumes"]):
            raise ValueError("Aufgenommene Volumes fehlen")
    if a["volumes_state"] != ("passed" if volumes else "unknown"):
        raise ValueError("Volumezustand widersprüchlich")
    return {"state": "validated_observation", "attempt_id": a["id"], "observed_at": result["observed_at"],
            "plan_json_sha256": plan_digest, "snapshot_sha256": snapshot_digest, "preflight_sha256": preflight_digest,
            "identity": "same", "reboot": reboot, "services": "active_state_only", "volumes": "recorded_membership_only" if volumes else "not_applicable_no_recorded_storage",
            "trust_limit": "Vertraute private Dateien, SSH/root/Helfer und Operatorzuordnung; keine Signatur oder Attestierung des ausgeführten Helferbuilds."}
