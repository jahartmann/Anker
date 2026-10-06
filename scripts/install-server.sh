#!/bin/sh
# Install on a dedicated Linux Anker server and open the guided setup.
set -eu
if [ "${1:-}" = --help ]; then
 echo 'sudo ./scripts/install-server.sh [BINARY] [--no-setup]'
 echo 'Ohne BINARY wird die passende Datei aus dem Release beziehungsweise bin/ gewählt.'
 exit 0
fi
[ "$(id -u)" -eq 0 ] || { echo 'Als root auf dem Anker-Server ausführen.' >&2; exit 1; }
[ "$(uname -s)" = Linux ] || { echo 'Nur Linux wird unterstützt.' >&2; exit 1; }
ANKER_SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ANKER_SETUP=yes
ANKER_BINARY=
for ANKER_ARGUMENT in "$@"; do
 case "$ANKER_ARGUMENT" in
  --no-setup) ANKER_SETUP=no ;;
  --*) echo 'Unbekannte Option. --help verwenden.' >&2; exit 1 ;;
  *) [ -z "$ANKER_BINARY" ] || { echo 'Nur einen Binarypfad angeben.' >&2; exit 1; }; ANKER_BINARY=$ANKER_ARGUMENT ;;
 esac
done
case "$(uname -m)" in
 x86_64) ANKER_ARCH=amd64 ;;
 aarch64|arm64) ANKER_ARCH=arm64 ;;
 *) echo 'Unterstützt werden amd64 und arm64.' >&2; exit 1 ;;
esac
if [ -z "$ANKER_BINARY" ]; then
 if [ -f "$ANKER_SOURCE/anker" ]; then ANKER_BINARY="$ANKER_SOURCE/anker"
 else ANKER_BINARY="$ANKER_SOURCE/bin/anker-linux-$ANKER_ARCH"; fi
fi
for ANKER_DEPENDENCY in systemctl useradd runuser install ssh-keygen python3; do
 command -v "$ANKER_DEPENDENCY" >/dev/null || { echo "Fehlendes Programm: $ANKER_DEPENDENCY" >&2; exit 1; }
done
[ -d /run/systemd/system ] || { echo 'Ein laufendes systemd-System wird benötigt.' >&2; exit 1; }
[ -f "$ANKER_BINARY" ] && [ -x "$ANKER_BINARY" ] || { echo 'Passendes Release entpacken oder zuerst make linux ausführen.' >&2; exit 1; }
# Detect the wrong architecture before writing anything into the installation.
"$ANKER_BINARY" version
if [ "$ANKER_SETUP" = yes ] && [ ! -t 0 ]; then
 echo 'Die Einrichtung benötigt ein Terminal. Für automatisierte Installation --no-setup verwenden.' >&2; exit 1
fi
if [ -e /usr/local/bin/anker ]; then
 if [ ! -f /etc/anker/install-pending ]; then
  echo 'Anker ist bereits installiert. Einrichtung: anker setup; Updates: anker update install.' >&2
  exit 1
 fi
 if systemctl is-active --quiet anker.service; then
  echo 'Anker läuft bereits; unvollständige Installation zuerst im Wartungsfenster prüfen.' >&2; exit 1
 fi
fi
python3 "$ANKER_SOURCE/scripts/install-state.py" begin
id anker >/dev/null 2>&1 || useradd --system --home-dir /srv/anker --shell /usr/sbin/nologin anker
install -d -o anker -g anker -m 0700 /srv/anker
install -d -o root -g anker -m 0750 /etc/anker /etc/anker/keys
install -d -o root -g root -m 0755 /usr/local/libexec
install -d -o root -g root -m 0755 /usr/local/bin
ANKER_STAGED=$(mktemp /usr/local/bin/.anker-install-XXXXXXXX)
trap 'rm -f "$ANKER_STAGED"' EXIT HUP INT TERM
install -o root -g root -m 0755 "$ANKER_BINARY" "$ANKER_STAGED"
install -o root -g root -m 0755 "$ANKER_BINARY" /usr/local/libexec/anker-updater
install -o root -g root -m 0644 "$ANKER_SOURCE/deploy/anker.service" /etc/systemd/system/anker.service
install -o root -g root -m 0644 "$ANKER_SOURCE/deploy/anker-updater.service" /etc/systemd/system/anker-updater.service
if [ ! -e /etc/anker/service.env ]; then
 printf '%s\n' 'ANKER_LISTEN=127.0.0.1:8087' >/etc/anker/service.env
 chown root:anker /etc/anker/service.env; chmod 0640 /etc/anker/service.env
fi
if [ -f "$ANKER_SOURCE/public.key" ]; then
 install -o root -g anker -m 0640 "$ANKER_SOURCE/public.key" /etc/anker/release-public.key
fi
systemctl daemon-reload
python3 "$ANKER_SOURCE/scripts/install-state.py" finish "$ANKER_STAGED"
if [ "$ANKER_SETUP" = yes ]; then exec /usr/local/bin/anker setup; fi
printf '%s\n' 'Installiert. Einrichtung starten: sudo anker setup'
