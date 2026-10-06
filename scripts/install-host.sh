#!/bin/sh
# Install the helper and optionally its restricted public keys in one step.
set -eu
if [ "${1:-}" = --help ]; then
 cat <<'HELP'
Auf jedem Proxmox-Host als root ausführen:
  ./scripts/install-host.sh --backup-key /pfad/backup.pub
  ./scripts/install-host.sh --backup-key /pfad/backup.pub --restore-key /pfad/restore.pub
Ohne Schlüsseldateien werden nur Helfer und Zugänge vorbereitet.
--restore-key aktiviert den separaten Wiederherstellungszugang.
--enable-restore bereitet diesen Zugang ohne automatischen Schlüsseleintrag vor.
Nur öffentliche Ed25519-Schlüssel verwenden. Vorhandene Profile und andere Schlüssel bleiben erhalten.
HELP
 exit 0
fi
ANKER_BACKUP_KEY=
ANKER_RESTORE_KEY=
ANKER_ENABLE_RESTORE=no
while [ "$#" -gt 0 ]; do
 case "$1" in
  --backup-key) [ "$#" -ge 2 ] || { echo 'Schlüsseldatei fehlt.' >&2; exit 1; }; ANKER_BACKUP_KEY=$2; shift 2 ;;
  --restore-key) [ "$#" -ge 2 ] || { echo 'Schlüsseldatei fehlt.' >&2; exit 1; }; ANKER_RESTORE_KEY=$2; ANKER_ENABLE_RESTORE=yes; shift 2 ;;
  --enable-restore) ANKER_ENABLE_RESTORE=yes; shift ;;
  *) echo 'Unbekannte Option; --help verwenden.' >&2; exit 1 ;;
 esac
done
[ "$(id -u)" -eq 0 ] || { echo 'Als root auf dem Proxmox-Host ausführen.' >&2; exit 1; }
for ANKER_DEPENDENCY in pveversion python3 useradd getent install visudo ssh-keygen; do
 command -v "$ANKER_DEPENDENCY" >/dev/null || { echo "Fehlendes Programm: $ANKER_DEPENDENCY" >&2; exit 1; }
done
ANKER_SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
set --
[ -z "$ANKER_BACKUP_KEY" ] || set -- "$@" --backup-key "$ANKER_BACKUP_KEY"
[ -z "$ANKER_RESTORE_KEY" ] || set -- "$@" --restore-key "$ANKER_RESTORE_KEY"
# Reject malformed, private or shared role keys before changing this host.
python3 "$ANKER_SOURCE/scripts/host-authorize.py" --validate-only "$@"
[ -f "$ANKER_SOURCE/host/anker_host.py" ] || { echo 'Hosthelfer im Paket fehlt.' >&2; exit 1; }
for ANKER_ACCOUNT in anker anker-restore; do
 [ "$ANKER_ACCOUNT" = anker ] || [ "$ANKER_ENABLE_RESTORE" = yes ] || continue
 ANKER_EXPECTED_HOME=/var/lib/anker-ssh
 [ "$ANKER_ACCOUNT" = anker ] || ANKER_EXPECTED_HOME=/var/lib/anker-restore-ssh
 if getent passwd "$ANKER_ACCOUNT" >/dev/null; then
  ANKER_ACTUAL_HOME=$(getent passwd "$ANKER_ACCOUNT" | cut -d: -f6)
  ANKER_ACTUAL_UID=$(id -u "$ANKER_ACCOUNT")
  [ "$ANKER_ACTUAL_HOME" = "$ANKER_EXPECTED_HOME" ] && [ "$ANKER_ACTUAL_UID" -ne 0 ] || { echo "Vorhandener Benutzer $ANKER_ACCOUNT gehört nicht zur Hosteinrichtung; zentrale Anker-Installation auf einem getrennten Server betreiben." >&2; exit 1; }
 fi
 [ ! -L "$ANKER_EXPECTED_HOME" ] && [ ! -L "$ANKER_EXPECTED_HOME/.ssh" ] || { echo 'SSH-Zugangsordner dürfen keine symbolischen Links sein.' >&2; exit 1; }
done
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
install -d -o root -g root -m 0755 /var/lib/anker-ssh /var/lib/anker-ssh/.ssh
if [ ! -e /var/lib/anker-ssh/.ssh/authorized_keys ]; then
 touch /var/lib/anker-ssh/.ssh/authorized_keys
 chown root:root /var/lib/anker-ssh/.ssh/authorized_keys; chmod 0644 /var/lib/anker-ssh/.ssh/authorized_keys
fi
if [ "$ANKER_ENABLE_RESTORE" = yes ]; then
 id anker-restore >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/anker-restore-ssh --shell /bin/sh anker-restore
 printf '%s\n' 'anker-restore ALL=(root) NOPASSWD: /usr/local/lib/anker/anker-host ""' >/etc/sudoers.d/anker-restore
 chmod 0440 /etc/sudoers.d/anker-restore; visudo -cf /etc/sudoers.d/anker-restore
 install -d -o root -g root -m 0755 /var/lib/anker-restore-ssh /var/lib/anker-restore-ssh/.ssh
 if [ ! -e /var/lib/anker-restore-ssh/.ssh/authorized_keys ]; then
  touch /var/lib/anker-restore-ssh/.ssh/authorized_keys
  chown root:root /var/lib/anker-restore-ssh/.ssh/authorized_keys; chmod 0644 /var/lib/anker-restore-ssh/.ssh/authorized_keys
 fi
fi
python3 "$ANKER_SOURCE/scripts/host-authorize.py" "$@"
printf '%s\n' 'Hosthelfer installiert. Angegebene öffentliche Schlüssel sind mit festem Helferkommando eingetragen.'
if [ -f /etc/ssh/ssh_host_ed25519_key.pub ]; then
 printf '%s\n' 'SSH-Hostfingerprint für die Prüfung auf dem Anker-Server:'
 ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
 printf '%s\n' 'Auf dem Anker-Server: sudo anker host trust ADRESSE --fingerprint SHA256:...'
else
 printf '%s\n' 'Kein Ed25519-Hostschlüssel gefunden. OpenSSH-Hostschlüssel und bekannte Hosteinträge manuell prüfen.'
fi
