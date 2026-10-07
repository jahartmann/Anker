#!/bin/sh
# Install on a dedicated Linux Anker server and open the guided setup.
set -eu
if [ "${1:-}" = --help ]; then
 echo 'sudo ./scripts/install-server.sh [BINARY] [--no-setup]'
 echo 'Ohne BINARY wird aus dem Checkout automatisch gebaut oder die Release-Datei verwendet.'
 echo 'Erkennt vorhandene Installationen und aktualisiert sie mit Sicherung und Startprüfung.'
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
[ -d /run/systemd/system ] || { echo 'Ein laufendes systemd-System wird benötigt.' >&2; exit 1; }
if [ "$ANKER_SETUP" = yes ] && [ ! -t 0 ]; then
 echo 'Die Einrichtung benötigt ein Terminal. Für automatisierte Installation --no-setup verwenden.' >&2; exit 1
fi
ANKER_BUILD=no
if [ -z "$ANKER_BINARY" ]; then
 if [ -f "$ANKER_SOURCE/go.mod" ]; then ANKER_BUILD=yes
 elif [ -f "$ANKER_SOURCE/anker" ]; then ANKER_BINARY="$ANKER_SOURCE/anker"
 else ANKER_BINARY="$ANKER_SOURCE/bin/anker-linux-$ANKER_ARCH"; fi
fi
ANKER_PACKAGES=
for ANKER_PAIR in systemctl:systemd useradd:passwd runuser:util-linux install:coreutils ssh-keygen:openssh-client python3:python3 flock:util-linux openssl:openssl rsync:rsync; do
 ANKER_COMMAND=${ANKER_PAIR%%:*}; ANKER_PACKAGE=${ANKER_PAIR#*:}
 if ! command -v "$ANKER_COMMAND" >/dev/null; then ANKER_PACKAGES="$ANKER_PACKAGES $ANKER_PACKAGE"; fi
done
if [ "$ANKER_BUILD" = yes ]; then
 command -v go >/dev/null || ANKER_PACKAGES="$ANKER_PACKAGES golang-go"
 # The compiler and modules are downloaded over verified TLS.
 if command -v dpkg-query >/dev/null && ! dpkg-query -W -f='${Status}' ca-certificates 2>/dev/null | grep -q 'install ok installed'; then
  ANKER_PACKAGES="$ANKER_PACKAGES ca-certificates"
 fi
fi
if [ -n "$ANKER_PACKAGES" ]; then
 command -v apt-get >/dev/null || { echo "Fehlende Werkzeuge installieren: $ANKER_PACKAGES" >&2; exit 1; }
 echo 'Anker · fehlende Systempakete installieren'
 apt-get update
 # Package names come only from the fixed list above.
 DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends $ANKER_PACKAGES
fi
umask 077
exec 9>/run/anker-install.lock
flock -n 9 || { echo 'Eine Installation läuft bereits.' >&2; exit 1; }
ANKER_BUILD_DIR=
ANKER_STAGED=
cleanup() { [ -z "$ANKER_BUILD_DIR" ] || rm -rf "$ANKER_BUILD_DIR"; [ -z "$ANKER_STAGED" ] || rm -f "$ANKER_STAGED"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
if [ "$ANKER_BUILD" = yes ]; then
 echo 'Anker · Programm bauen (Weboberfläche ist bereits enthalten)'
 ANKER_BUILD_DIR=$(mktemp -d "$ANKER_SOURCE/.anker-build-XXXXXXXX")
 ANKER_BINARY="$ANKER_BUILD_DIR/anker"
 (cd "$ANKER_SOURCE" && CGO_ENABLED=0 GOTOOLCHAIN=auto go build -trimpath -p 2 -o "$ANKER_BINARY" ./cmd/anker)
fi
[ -f "$ANKER_BINARY" ] && [ -x "$ANKER_BINARY" ] || { echo 'Passende Linux-Binärdatei fehlt; einen Quellcode-Checkout oder ein geprüftes Release verwenden.' >&2; exit 1; }
# Detect the wrong architecture before writing anything into the installation.
"$ANKER_BINARY" version
if [ -e /usr/local/bin/anker ] && [ ! -f /etc/anker/install-pending ]; then
 "$ANKER_BINARY" install-local
 cleanup
 trap - EXIT HUP INT TERM
 exec 9>&-
 if [ "$ANKER_SETUP" = yes ]; then exec /usr/local/bin/anker setup; fi
 echo 'Aktualisiert. Einrichtung bei Bedarf: anker setup'
 exit 0
fi
if [ -e /usr/local/bin/anker ] && systemctl is-active --quiet anker.service; then
 echo 'Unvollständige Erstinstallation läuft bereits; im Wartungsfenster prüfen.' >&2; exit 1
fi
python3 "$ANKER_SOURCE/scripts/install-state.py" begin
id anker >/dev/null 2>&1 || useradd --system --home-dir /srv/anker --shell /usr/sbin/nologin anker
install -d -o anker -g anker -m 0700 /srv/anker
install -d -o root -g anker -m 0750 /etc/anker /etc/anker/keys
install -d -o root -g root -m 0755 /usr/local/libexec
install -d -o root -g root -m 0755 /usr/local/bin
ANKER_STAGED=$(mktemp /usr/local/bin/.anker-install-XXXXXXXX)

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
cleanup
trap - EXIT HUP INT TERM
exec 9>&-
if [ "$ANKER_SETUP" = yes ]; then exec /usr/local/bin/anker setup; fi
printf '%s\n' 'Installiert. Einrichtung starten: sudo anker setup'
