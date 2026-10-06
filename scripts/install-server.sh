#!/bin/sh
# Install only on a dedicated Linux Anker server. Does not initialize credentials.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Als root auf dem Anker-Server ausführen.' >&2; exit 1; }
[ "$(uname -s)" = Linux ] || { echo 'Nur Linux wird unterstützt.' >&2; exit 1; }
ANKER_BINARY=${1:?Pfad zum passenden Linux-Binary angeben}
ANKER_SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ -e /usr/local/bin/anker ]; then
 echo 'Anker ist bereits installiert. Für Updates anker update install verwenden.' >&2
 exit 1
fi
id anker >/dev/null 2>&1 || useradd --system --home-dir /srv/anker --shell /usr/sbin/nologin anker
install -d -o anker -g anker -m 0700 /srv/anker
install -d -o root -g anker -m 0750 /etc/anker /etc/anker/keys
install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "$ANKER_BINARY" /usr/local/bin/anker
install -o root -g root -m 0755 "$ANKER_BINARY" /usr/local/libexec/anker-updater
install -o root -g root -m 0644 "$ANKER_SOURCE/deploy/anker.service" /etc/systemd/system/anker.service
install -o root -g root -m 0644 "$ANKER_SOURCE/deploy/anker-updater.service" /etc/systemd/system/anker-updater.service
if [ ! -e /etc/anker/service.env ]; then
 printf '%s\n' 'ANKER_LISTEN=127.0.0.1:8087' >/etc/anker/service.env
 chown root:anker /etc/anker/service.env; chmod 0640 /etc/anker/service.env
fi
systemctl daemon-reload
printf '%s\n' 'Installiert. Jetzt Administrator initialisieren, SSH-Zugang einrichten und danach systemctl enable --now anker ausführen. Siehe README.'
