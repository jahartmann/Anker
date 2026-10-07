#!/bin/sh
# Destructive fixed-path integration checks, exclusively on an empty GitHub runner.
set -eu
[ "${GITHUB_ACTIONS:-}" = true ] && [ "$(id -u)" -eq 0 ] || { echo 'Nur im isolierten root-CI-Harness ausführen.' >&2; exit 1; }
[ -d /run/systemd/system ] || { echo 'systemd ist nicht aktiv.' >&2; exit 1; }
[ ! -e /usr/local/bin/anker ] && [ ! -e /srv/anker ] && [ ! -e /etc/anker ] || { echo 'Keine bestehende Anker-Installation verändern.' >&2; exit 1; }
ANKER_TEST_BINARY=${1:?Testbinary angeben}
ANKER_TEST_SUITE=${2:?Kompilierte Updater-Tests angeben}
ANKER_TEST_CANDIDATE=${3:?Passende neue Testversion angeben}
cleanup() {
  ANKER_TEST_EXIT=$?
  if [ "$ANKER_TEST_EXIT" -ne 0 ]; then
    systemctl show anker-updater --property=Result --property=StartLimitBurst --property=StartLimitIntervalUSec || true
    journalctl -u anker-updater -n 60 --no-pager || true
  fi
  systemctl stop anker-updater anker || true
  exit "$ANKER_TEST_EXIT"
}
trap cleanup EXIT
# Simulate a previously interrupted first installation and verify the retry.
python3 scripts/install-state.py begin
install -d /usr/local/bin
printf '%s\n' 'interrupted initial binary' >/usr/local/bin/anker
# Run the same no-binary command as a fresh clone; this also covers bootstrap.
./scripts/install-server.sh --no-setup
# Repeated installation before any administrator/catalog exists must work too.
./scripts/install-server.sh "$ANKER_TEST_BINARY" --no-setup
[ ! -e /srv/anker/catalog.db ] || { echo 'Katalog vor Einrichtung unerwartet angelegt.' >&2; exit 1; }
[ ! -e /etc/anker/install-pending ] || { echo 'Installationsmarker nicht abgeschlossen.' >&2; exit 1; }
# Reproduce a dedicated ext filesystem mounted directly at the data root.
install -d -o root -g root -m 0700 /srv/anker/lost+found
printf '%s\n' 'recovered filesystem data' >/srv/anker/lost+found/recovered-file
chmod 0600 /srv/anker/lost+found/recovered-file
python3 - <<'PY'
import base64,os,pathlib
key=base64.b64encode(bytes(range(32)))+b'\n'
path=pathlib.Path('/etc/anker/release-public.key');path.write_bytes(key);path.chmod(0o640)
os.chown(path,0,__import__('grp').getgrnam('anker').gr_gid)
PY
python3 scripts/test-setup-pty.py
python3 - <<'PY'
import pathlib,stat
directory=pathlib.Path('/srv/anker/lost+found');info=directory.stat()
assert info.st_uid==0 and info.st_gid==0 and stat.S_IMODE(info.st_mode)==0o700,'filesystem recovery permissions changed'
assert (directory/'recovered-file').read_text()=='recovered filesystem data\n','filesystem recovery data changed'
assert pathlib.Path('/srv/anker/.anker-mode').read_text()=='production\n','production initialization failed'
print('Setup with inaccessible root-owned lost+found passed; recovery directory preserved.')
PY
# Repeat the installer against real running services, preserving login and keys.
python3 - <<'PY'
import pathlib,subprocess,urllib.request,json
before={path:pathlib.Path(path).read_bytes() for path in ['/etc/anker/keys/backup','/etc/anker/keys/restore','/etc/anker/service.env']}
subprocess.run(['systemctl','reset-failed','anker','anker-updater'],check=True)
subprocess.run(['./scripts/install-server.sh','--no-setup'],check=True)
for path,content in before.items():assert pathlib.Path(path).read_bytes()==content,'Repeated installer changed '+path
request=urllib.request.Request('http://127.0.0.1:8087/api/login',data=json.dumps({'name':'admin','password':'init2026'}).encode(),headers={'Content-Type':'application/json','X-Anker-Request':'1'})
with urllib.request.urlopen(request) as response:assert response.status==200
subprocess.run(['systemctl','is-active','--quiet','anker','anker-updater'],check=True)
assert not pathlib.Path('/var/lib/anker-updater/pending.json').exists(),'Live installer journal not completed'
print('Automatic source build and repeated live installation preserved authentication and SSH identities.')
PY
touch /run/anker-isolated-ci
ANKER_SYSTEMD_TEST=1 ANKER_SYSTEMD_CANDIDATE="$ANKER_TEST_CANDIDATE" "$ANKER_TEST_SUITE" -test.run '^(TestSystemdInstallationAndRecovery|TestStorageRealExt4GrowthUnderSystemd)$' -test.v -test.timeout 5m
