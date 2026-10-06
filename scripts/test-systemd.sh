#!/bin/sh
# Destructive fixed-path integration checks, exclusively on an empty GitHub runner.
set -eu
[ "${GITHUB_ACTIONS:-}" = true ] && [ "$(id -u)" -eq 0 ] || { echo 'Nur im isolierten root-CI-Harness ausführen.' >&2; exit 1; }
[ -d /run/systemd/system ] || { echo 'systemd ist nicht aktiv.' >&2; exit 1; }
[ ! -e /usr/local/bin/anker ] && [ ! -e /srv/anker ] && [ ! -e /etc/anker ] || { echo 'Keine bestehende Anker-Installation verändern.' >&2; exit 1; }
ANKER_TEST_BINARY=${1:?Testbinary angeben}
ANKER_TEST_SUITE=${2:?Kompilierte Updater-Tests angeben}
ANKER_TEST_CANDIDATE=${3:?Passende neue Testversion angeben}
trap 'systemctl stop anker-updater anker || true' EXIT
# Simulate a previously interrupted first installation and verify the retry.
python3 scripts/install-state.py begin
install -d /usr/local/bin
printf '%s\n' 'interrupted initial binary' >/usr/local/bin/anker
./scripts/install-server.sh "$ANKER_TEST_BINARY" --no-setup
[ ! -e /etc/anker/install-pending ] || { echo 'Installationsmarker nicht abgeschlossen.' >&2; exit 1; }
python3 - <<'PY'
import base64,os,pathlib
key=base64.b64encode(bytes(range(32)))+b'\n'
path=pathlib.Path('/etc/anker/release-public.key');path.write_bytes(key);path.chmod(0o640)
os.chown(path,0,__import__('grp').getgrnam('anker').gr_gid)
PY
python3 scripts/test-setup-pty.py
touch /run/anker-isolated-ci
ANKER_SYSTEMD_TEST=1 ANKER_SYSTEMD_CANDIDATE="$ANKER_TEST_CANDIDATE" "$ANKER_TEST_SUITE" -test.run '^TestSystemdInstallationAndRecovery$' -test.v -test.timeout 5m
