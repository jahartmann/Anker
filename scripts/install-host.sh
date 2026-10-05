#!/bin/sh
# Install the fixed helper; no credentials are generated or automatically trusted.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Als root auf dem Proxmox-Host ausführen.' >&2; exit 1; }
command -v pveversion >/dev/null
command -v python3 >/dev/null
ANKER_SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
id anker >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/anker-ssh --shell /bin/sh anker
install -d -o root -g root -m 0755 /usr/local/lib/anker
install -o root -g root -m 0755 "$ANKER_SOURCE/host/anker_host.py" /usr/local/lib/anker/anker-host
if [ ! -e /etc/anker-host.json ]; then
 printf '%s\n' '{"paths":["/etc","/usr/local"]}' >/etc/anker-host.json
 chmod 0600 /etc/anker-host.json
fi
printf '%s\n' 'anker ALL=(root) NOPASSWD: /usr/local/lib/anker/anker-host --read-only' >/etc/sudoers.d/anker
chmod 0440 /etc/sudoers.d/anker
visudo -cf /etc/sudoers.d/anker
install -d -o root -g root -m 0755 /var/lib/anker-ssh/.ssh
if [ ! -e /var/lib/anker-ssh/.ssh/authorized_keys ]; then
 touch /var/lib/anker-ssh/.ssh/authorized_keys
 chown root:root /var/lib/anker-ssh/.ssh/authorized_keys; chmod 0644 /var/lib/anker-ssh/.ssh/authorized_keys
fi
printf '%s\n' 'Helper installiert. Öffentlichen Schlüssel mit erzwungenem Kommando hinzufügen; SSH-Hostfingerprint unabhängig prüfen. Siehe README.'

if [ "${1:-}" = "--enable-restore" ]; then
 id anker-restore >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/anker-restore-ssh --shell /bin/sh anker-restore
 printf '%s\n' 'anker-restore ALL=(root) NOPASSWD: /usr/local/lib/anker/anker-host ""' >/etc/sudoers.d/anker-restore
 chmod 0440 /etc/sudoers.d/anker-restore; visudo -cf /etc/sudoers.d/anker-restore
 install -d -o root -g root -m 0755 /var/lib/anker-restore-ssh/.ssh
 touch /var/lib/anker-restore-ssh/.ssh/authorized_keys
 chown root:root /var/lib/anker-restore-ssh/.ssh/authorized_keys; chmod 0644 /var/lib/anker-restore-ssh/.ssh/authorized_keys
 printf '%s\n' 'Separater Restorezugang vorbereitet. Nur dedizierten Restore-Schlüssel mit erzwungenem Helferkommando eintragen.'
fi
