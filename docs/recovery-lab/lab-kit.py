#!/usr/bin/env python3
"""Operatorgeführte PVE-Abnahme; ausschließlich lokale, feste Leseabfragen.
Python 3, nur Standardbibliothek. Keine SSH-, Restore- oder Reboot-Ausführung.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import selectors
import subprocess
import time
import re
from verification import object_pairs, reject_constant, timestamp, private_bytes, validate_machine

FIELDS = (
    "lab_id operator date isolation_reference build_id helper_privileges_reference "
    "pve_expected source_backup_id source_fingerprint plan_id plan_package_sha256 "
    "target_id target_fingerprint baseline_boot_id backing_reference "
    "dependency_dossier_reference preflight_reference source_manifest_sha target_snapshot_sha "
    "connection_sha machine_sha256 build_sha256 helper_sha256 provenance_reference cold_package_sha256"
).split()
SCENARIOS = {
    1: [
        "Standalone: isoliertes Ziel, frische Sicherung/Plan-ID, ausgewählte gewöhnliche Datei und Ziel-vorher dokumentieren.",
        "HALT: Konsole/Rückweg, Platz und angehaltene fremde Schreiber prüfen; Operator bestätigt genau diesen Plan und übernimmt manuell über Anker.",
        "Inhalt/Metadaten und Journal mit Plan vergleichen. Rücksetzung separat bestätigen; Ziel-vorher und Rechte erneut prüfen.",
        "Dienstfunktion und frischen planbezogenen Maschinennachweis erfassen; fehlende Prüffähigkeit bleibt offen."
    ],
    2: [
        "Replacement: alten Host isolieren; neues PVE mit abweichenden NIC-Namen und Disks installieren. Boot/Root des Ziels erhalten.",
        "Ports nach physischer Funktion zuordnen; Disk-/Storage-Backing, passende Volumes, Gast-/PBS-Sicherung und Schlüsselverwahrort im Dossier belegen.",
        "HALT bei unbekanntem Backing, fehlenden Gästen oder Konsole. Operator prüft vorbereitete Dateien; Netz/Identität/hardwareabhängige Schritte einzeln manuell freigeben.",
        "Gastdaten/PBS separat operatorgeführt wiederherstellen; Management, Dienste und Gäste prüfen. Anker verwaltet keine Pools/Volumes."
    ],
    3: [
        "Zwei getrennte Durchläufe protokollieren: gesunder isolierter Cluster mit Ersatznode UND isolierter vollständiger Clusterverlust. Alle Node-IDs, PVE-Versionen und Isolation nachweisen.",
        "Gesunder Cluster: aktuellen gemeinsamen Zustand erhalten, alte config.db niemals einsetzen. Lokale Ziel-vorher-Daten sichern.",
        "HALT vor Node-Entfernung/Join, Quorum, Ceph und HA: freigegebener versionsgerechter manueller Ablauf durch Operator erforderlich.",
        "Vollständiger Verlust: alle alten Nodes ausschließen, gemeinsamen Sicherungsstand bestimmen. HALT vor manueller Rekonstruktion der Cluster-Datenbank; Stop-/Startreihenfolge nach passender PVE-Anleitung dokumentieren.",
        "Jeden Node, gemeinsamen Zustand, Storage, Gäste und Dienste getrennt belegen. Unklarer Teilablauf lässt das Gesamtszenario offen."
    ],
    4: [
        "Offline: Planpaket und unabhängig dokumentierten Paketdigest auf isolierten Arbeitsplatz übertragen; Anker-Dienst bleibt aus.",
        "anker plan-package verify --input PLAN.tar ausführen; JSON/Digest privat ablegen. complete belegt nur interne Abdeckung, limited bleibt begrenzt; Herkunft separat prüfen.",
        "HALT vor Transfer: Index, Anleitung, Manifestmetadaten, prepared-files und target-before prüfen. Originalsymlinks nur als Metadaten behandeln; Archiv nie direkt nach / entpacken.",
        "Ohne laufendes Anker ausgewählte gewöhnliche Datei mit lokalen Archivwerkzeugen in privater Staging-Ablage einzeln gewinnen; Operator überträgt manuell mit geprüftem SSH-Hostschlüssel. Vorherkopie sichern, Rechte/UID/GID/xattrs aus Manifest bewusst prüfen.",
        "Zielinhalt, Rechte, Dienste und manuellen Rückweg belegen; erfolgreiche Paketprüfung ist keine erfolgreiche Wiederherstellung."
    ],
    5: [
        "Nur im isolierten Labor: Plan und Ziel-vorher sichern; getrennte Durchläufe für Transferabbruch und verlorene Apply-Antwort vorbereiten.",
        "HALT: Operator unterbricht ausschließlich die Laborverbindung zum vorgesehenen Zeitpunkt. Keine automatische Wiederholung; aktiven Auftrag/Zielkonsole prüfen.",
        "Journalstatus über Anker abgleichen. Fehlendes Journal und Lese-/Transportfehler unterscheiden; dieselbe Plan-ID nicht blind erneut anwenden.",
        "Mit gültigem Rücksetzbeleg Rücksetzung separat bestätigen. Danach eigene Fremdänderung im Labor erzeugen und in getrenntem Durchlauf deren Überschreibungsverweigerung prüfen; unveränderten Fremdinhalt belegen."
    ],
    6: [
        "Quellmanifest und Sicherungsdigest vorab dokumentieren; ausschließlich entbehrliches isoliertes Testziel für Platzgrenze bereitstellen.",
        "HALT: Operator stellt kontrollierten Platzmangel her; niemals Produktivdatenträger füllen. Abbruch/Aufschub ohne Dateiübernahme und Fehlermeldung erfassen.",
        "Quelle erneut offline prüfen und Digests vergleichen; Ziel auf Teiländerungen prüfen. Platz nur manuell bereinigen; neuen Vergleich/Plan vor weiterem Versuch erstellen."
    ],
    7: [
        "Frischen Plan und alle Bindungen protokollieren. Getrennte Durchläufe für Quell-, Zielverbindungs-/Identitäts- und Konfigurationsdrift vorbereiten.",
        "HALT: Operator verändert nur eine entbehrliche Laborkopie oder kontrollierte Zielconfig, niemals das Originalbackup. Alte Entscheidung darf keine Übernahme erlauben.",
        "Driftablehnung dokumentieren; neue Quelle/Zielprüfung erzwingt neue Entscheidungen/Planbindung. Alte menschliche Aussagen und Belege nicht als neue Prüfung behandeln."
    ],
    8: [
        "Vorher Maschinenidentität, Plan-/Backup-ID, Boot-ID und Build/Helferdigest mit privatem Preflight erfassen; Konsole und Rückweg prüfen.",
        "HALT: Neustart ausschließlich ausdrücklich durch Operator in isolierter Umgebung. Dieses Kit sendet keinen Reboot.",
        "Nachher Preflight erneut mit neuer Datei erfassen. Gleiche Maschine und neue Boot-ID vergleichen; fehlende oder gleiche Boot-Baseline beweist keinen Neustart.",
        "anker restore verify PLAN ausführen; neueste verification_attempts mit privater Plan-Baseline und Nachherbeleg prüfen. Dienste/Management/Storage und Funktion separat dokumentieren."
    ],
    9: [
        "Anker-Katastrophe: ausschließlich Laborkopie; Anker-Dienst/Updater und alle Schreiber ausdrücklich durch Operator stoppen. Private Passphrase-Datei 0600 bereitstellen; keine Geheimnisse im Protokoll.",
        "HALT: Cold-Export gemäß README mit Datenroot und optional config-root erstellen; Paket separat sichern und inspect ausführen.",
        "Operator importiert ausschließlich in einen noch nicht vorhandenen isolierten Datenroot. Credentials privat auf Erhaltung vergleichen; nur Ergebnis/Digest, keine Inhalte protokollieren.",
        "Start muss durch .disaster-isolated gesperrt bleiben; Hosts disabled, Sessions verworfen, laufende/geplante Arbeit interrupted sowie alte Pläne/Inspections widerrufen prüfen. Private Evidenz bleibt lesbar/exportierbar.",
        "HALT: Marker nicht entfernen und alte Arbeit nicht fortsetzen. Explizite spätere lokale Migrationsprüfung ist ein eigener Ablauf; keine automatische Aktivierung."
    ]
}

def read_json(path):
    raw = private_bytes(path, 1 << 20)
    data = json.loads(raw, object_pairs_hook=object_pairs, parse_constant=reject_constant)
    if not isinstance(data, dict):
        raise ValueError("JSON-Objekt erforderlich")
    return data


def clean_bindings(data):
    allowed = set(FIELDS) | {"isolation_confirmed", "permitted_scenarios"}
    if set(data) - allowed:
        raise ValueError("Unbekannte Bindungsfelder; keine Geheimnisse oder Config-Inhalte eintragen")
    result = {k: data.get(k, "") for k in FIELDS}
    if any(not isinstance(v, str) or len(v) > 2048 for v in result.values()):
        raise ValueError("Bindungen müssen kurze Referenzen sein")
    result["isolation_confirmed"] = data.get("isolation_confirmed") is True
    scenarios = data.get("permitted_scenarios", [])
    if not isinstance(scenarios, list) or any(type(n) is not int or n not in SCENARIOS for n in scenarios):
        raise ValueError("Szenarien müssen Zahlen 1 bis 9 sein")
    result["permitted_scenarios"] = scenarios
    return result


def missing_bindings(data, number=None):
    required = set(FIELDS) - {"cold_package_sha256"}
    if number in (4, 5, 6, 7):
        required -= set("baseline_boot_id machine_sha256 target_snapshot_sha connection_sha preflight_reference".split())
    if number == 4:
        required -= {"helper_sha256", "helper_privileges_reference"}
    if number == 9:
        required = set("lab_id operator date isolation_reference build_id build_sha256 provenance_reference cold_package_sha256 dependency_dossier_reference".split())
    missing = [k for k in sorted(required) if not data.get(k, "").strip()]
    if not data.get("isolation_confirmed"):
        missing.append("isolation_confirmed")
    if not data.get("permitted_scenarios"):
        missing.append("permitted_scenarios")
    return missing


def capture(command, maximum=16384, timeout=10):
    """Fixed read commands only; bounded output/time, never collect stderr."""
    try:
        proc = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                stdin=subprocess.DEVNULL, env={"PATH":"/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL":"C"})
    except OSError:
        return {"state":"incomplete", "reason":"Befehl nicht verfügbar"}
    output = bytearray()
    deadline = time.monotonic() + timeout
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ)
    try:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                return {"state":"incomplete", "reason":"Zeitgrenze"}
            events = selector.select(remaining)
            if not events:
                return {"state":"incomplete", "reason":"Zeitgrenze"}
            chunk = os.read(proc.stdout.fileno(), min(4096, maximum + 1 - len(output)))
            if not chunk:
                code = proc.wait(timeout=max(0.01, deadline-time.monotonic()))
                if code != 0:
                    return {"state":"incomplete", "reason":"Abfrage fehlgeschlagen", "exit_code":code}
                return {"state":"ok", "value":output.decode("utf-8", "replace").strip()}
            output.extend(chunk)
            if len(output) > maximum:
                return {"state":"incomplete", "reason":"Ausgabegrenze"}
    except (OSError, subprocess.TimeoutExpired):
        return {"state":"incomplete", "reason":"Abfrage abgebrochen"}
    finally:
        selector.close()
        if proc.poll() is None:
            proc.kill()
        proc.wait()
        proc.stdout.close()


def digest_file(path):
    if path is None:
        return ""
    path = Path(path)
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 256 << 20:
        raise ValueError("Build/Helfer muss gewöhnliche Datei bis 256 MiB sein")
    h = hashlib.sha256()
    with path.open("rb") as stream:
        total = 0
        while True:
            b = stream.read(65536)
            if not b:
                break
            total += len(b)
            if total > 256 << 20:
                raise ValueError("Build/Helfer größer als 256 MiB")
            h.update(b)
    return h.hexdigest()


def read_identity():
    result = {}
    for label, path in [("machine_sha256", "/etc/machine-id"), ("boot_id", "/proc/sys/kernel/random/boot_id")]:
        try:
            with open(path, "rb") as stream:
                raw = stream.read(257).strip()
            if label == "machine_sha256":
                if not re.fullmatch(b"[0-9a-f]{32}", raw) or raw == b"0" * 32:
                    continue
                result[label] = hashlib.sha256(raw).hexdigest()
            else:
                value = raw.decode("ascii")
                if re.fullmatch(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}", value):
                    result[label] = value
        except (OSError, UnicodeError):
            continue
    return result


def preflight(bindings, build, helper, collect):
    bindings = clean_bindings(bindings)
    report = {"schema":1, "kind":"preflight", "status":"incomplete", "captured_at":datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "bindings":bindings, "missing":missing_bindings(bindings), "machine_verification":"not_collected",
              "build_sha256":digest_file(build), "helper_sha256":digest_file(helper), "observations":{},
              "residual_failures":["Reale Abnahme offen; Isolation/Backing/Helferrechte operatorseitig prüfen.",
                                   "Lokaler Preflight ersetzt keine frische planbezogene Maschinenprüfung."]}
    if not collect or report["missing"] or not report["build_sha256"] or not report["helper_sha256"]:
        report["residual_failures"].append("Lokale PVE-Erfassung nicht freigegeben oder Bindungen/Artefakte fehlen.")
        return report
    report["collection_authorized"] = True
    report["identity"] = read_identity()
    commands = {"pve_version":["pveversion"], "kernel":["uname", "-r"],
                "pve_cluster":["systemctl", "is-active", "pve-cluster"],
                "pvedaemon":["systemctl", "is-active", "pvedaemon"],
                "pveproxy":["systemctl", "is-active", "pveproxy"]}
    report["observations"] = {name:capture(command) for name,command in commands.items()}
    try:
        stat = os.statvfs("/var/lib/anker-host")
        report["rollback_available_bytes"] = stat.f_bavail * stat.f_frsize
    except OSError:
        report["residual_failures"].append("Freier Platz der Rollbackablage unbekannt.")
    # Values are observations, never proof of isolation, privileges or recovery.
    return report


def scenario_report(bindings, number):
    bindings = clean_bindings(bindings)
    if number not in bindings["permitted_scenarios"]:
        raise ValueError("Szenario nicht im Laborumfang freigegeben")
    return {"schema":1, "kind":"scenario", "scenario":number, "status":"incomplete", "bindings":bindings,
            "machine_verification":"not_collected", "residual_failures":[],
            "steps":[{"instruction":text, "outcome":"unrun", "actor":"", "date":"", "evidence_reference":""} for text in SCENARIOS[number]]}


def review(report, artifacts=(), machine_paths=()):
    number = report.get("scenario")
    if report.get("schema") != 1 or report.get("kind") != "scenario" or number not in SCENARIOS:
        raise ValueError("Unbekanntes Szenarioprotokoll")
    bindings = clean_bindings(report.get("bindings", {}))
    if number not in bindings["permitted_scenarios"]:
        raise ValueError("Szenario nicht freigegeben")
    steps = report.get("steps", [])
    if len(steps) != len(SCENARIOS[number]) or any(step.get("instruction") != expected for step,expected in zip(steps,SCENARIOS[number])):
        raise ValueError("Checkliste fehlt oder wurde verändert")
    missing = missing_bindings(bindings, number)
    if len(artifacts) > 32:
        raise ValueError("Höchstens 32 Belegdateien je Dossier")
    artifacts_by_sha = {hashlib.sha256(private_bytes(path, 256 << 20)).hexdigest() for path in artifacts}
    open_steps = []
    for i, step in enumerate(steps):
        if step.get("outcome") not in ("unrun", "pass", "fail"):
            raise ValueError("Unbekanntes Schrittergebnis")
        try:
            timestamp(step.get("date"))
            dated = timestamp(bindings["date"]) <= timestamp(step["date"]) <= datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(minutes=5)
        except (ValueError, TypeError):
            dated = False
        if step.get("outcome") != "pass" or not step.get("actor") or not dated or step.get("evidence_reference") not in artifacts_by_sha:
            open_steps.append(i+1)
    residual = report.get("residual_failures", [])
    if not isinstance(residual, list) or any(not isinstance(v, str) for v in residual):
        raise ValueError("Restfehler müssen eine Textliste sein")
    machine = {"state": "not_applicable", "reason": "Offline-/Fehler-/Cold-Szenario: Operatorbelege erforderlich; keine alte Planfreigabe/SSH-Abfrage."}
    if number in (1, 2, 3, 8):
        machine = {"state": "missing"}
        if len(machine_paths) == 3 and all(machine_paths):
            machine = validate_machine(bindings, number, *machine_paths)
    elif machine_paths and any(machine_paths):
        raise ValueError("Dieses Szenario verlangt keine frische Plan-SSH-Prüfung")
    failed = any(s.get("outcome") == "fail" for s in steps)
    complete = not missing and not open_steps and not any(v.strip() for v in residual) and machine["state"] != "missing"
    if number != 9 and bindings.get("plan_package_sha256") not in artifacts_by_sha:
        complete = False
        missing.append("plan_package_artifact")
    if number == 9:
        for key in ("cold_package_sha256", "build_sha256"):
            if bindings.get(key) not in artifacts_by_sha:
                complete = False
                missing.append(key + "_artifact")
    status = "failed" if failed else "complete" if complete else "incomplete"
    return {"status":status, "scenario":number, "bindings":bindings, "missing":missing,
            "machine_verification":machine, "note":"Vollständig bedeutet Belegdossier, keine universelle Recovery-Freigabe. Menschliche Bewertungen bleiben menschlich; Dateidigests beweisen keine Herkunft.",
            "open_steps":open_steps, "residual_failures":residual}


def save_private(path, report):
    # Existing files/symlinks are never overwritten. Use a private operator-owned parent.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    p = sub.add_parser("preflight")
    p.add_argument("--bindings", required=True)
    p.add_argument("--output", required=True)
    p.add_argument("--build")
    p.add_argument("--helper")
    p.add_argument("--collect-local-pve", action="store_true")
    p = sub.add_parser("scenario")
    p.add_argument("--bindings", required=True)
    p.add_argument("--number", required=True, type=int, choices=range(1,10))
    p.add_argument("--output", required=True)
    p.add_argument("--interactive", action="store_true")
    p = sub.add_parser("review")
    p.add_argument("--input", required=True)
    p.add_argument("--artifact", action="append", default=[], help="Privater tatsächlicher Beleg; pro Schritt dessen SHA256 als evidence_reference")
    p.add_argument("--plan", help="Tatsächliche JSON-Ausgabe von anker restore verify PLAN")
    p.add_argument("--snapshot", help="Bytegenaue private target-snapshot.json des Plans")
    p.add_argument("--preflight", help="Separater gebundener lokaler Preflight")
    p.add_argument("--output", help="Neue private Datei für das validierte Dossier")
    args = parser.parse_args()
    if args.command == "preflight":
        report = preflight(read_json(args.bindings), args.build, args.helper, args.collect_local_pve)
        save_private(args.output, report)
    elif args.command == "scenario":
        report = scenario_report(read_json(args.bindings), args.number)
        if args.interactive:
            print("Nur Protokollierung. Änderungen führt ausschließlich der Operator separat aus.")
            for step in report["steps"]:
                print(step["instruction"])
                choice = input("Ergebnis pass/fail/offen (leer = offen): ").strip()
                step["outcome"] = choice if choice in ("pass", "fail") else "unrun"
                if choice in ("pass", "fail"):
                    step["actor"] = input("Person: ").strip()
                    step["date"] = input("Datum/Zeit (UTC): ").strip()
                    step["evidence_reference"] = input("Private Belegreferenz/Digest (keine Inhalte): ").strip()
            report["residual_failures"] = [input("Restfehler/offene Punkte: ").strip()]
        report["status"] = review(report)["status"]
        save_private(args.output, report)
    else:
        report = review(read_json(args.input), args.artifact, (args.plan, args.snapshot, args.preflight))
        if args.output:
            save_private(args.output, report)
        print(json.dumps(report, ensure_ascii=False, indent=2))
    print("Status:", report["status"], "· Belegdossier; kein universeller Recovery-Pass.")
    return {"complete": 0, "failed": 1}.get(report["status"], 2)

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, TypeError, KeyError, AttributeError, RecursionError, EOFError, KeyboardInterrupt) as error:
        print("Laborprotokoll unvollständig:", type(error).__name__)
        raise SystemExit(2)
