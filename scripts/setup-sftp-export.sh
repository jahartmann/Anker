#!/bin/sh
# Optional read-only export of the existing backup tree. Run on the Anker server.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Als root ausführen.' >&2; exit 1; }
ANKER_EXPORT_KEY=${1:?Datei mit genau einem öffentlichen OpenSSH-Schlüssel angeben}
python3 - "$ANKER_EXPORT_KEY" <<'PY'
import pathlib,sys
lines=pathlib.Path(sys.argv[1]).read_text().strip().splitlines()
if len(lines)!=1 or not lines[0].startswith(('ssh-ed25519 ','ssh-rsa ','ecdsa-sha2-')): raise SystemExit('Ungültiger öffentlicher Schlüssel')
PY
ANKER_UID=$(id -u anker)
id anker-export >/dev/null 2>&1 || useradd --system --non-unique --uid "$ANKER_UID" --gid anker --home-dir / --shell /usr/sbin/nologin anker-export
[ "$(id -u anker-export)" = "$ANKER_UID" ] || { echo 'UID von anker-export stimmt nicht überein.' >&2; exit 1; }
install -d -o root -g root -m 0755 /srv/anker-sftp /srv/anker-sftp/hosts /srv/anker-sftp/plans
install -o root -g root -m 0644 "$ANKER_EXPORT_KEY" /etc/ssh/anker-export.keys
for ANKER_TREE in hosts plans; do
 mountpoint -q "/srv/anker-sftp/$ANKER_TREE" || mount --bind "/srv/anker/$ANKER_TREE" "/srv/anker-sftp/$ANKER_TREE"
 mount -o remount,bind,ro,nodev,nosuid,noexec "/srv/anker-sftp/$ANKER_TREE"
done
cat > /etc/ssh/sshd_config.d/anker-export.conf <<'CONF'
Match User anker-export
    ChrootDirectory /srv/anker-sftp
    AuthorizedKeysFile /etc/ssh/anker-export.keys
    ForceCommand internal-sftp -R -d /hosts
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    AllowTcpForwarding no
    AllowAgentForwarding no
    X11Forwarding no
    PermitTunnel no
    PermitTTY no
Match all
CONF
sshd -t
printf '%s\n' 'SFTP-Konfiguration geprüft. Erst nach Prüfung der effektiven Konfiguration SSH neu laden.' 'Bind-Mounts sind bis zum Neustart aktiv. Persistente Mounts und Startreihenfolge gemäß README einrichten.'
