#!/usr/bin/env python3
"""Real SSH onboarding through the running Web/root services on the isolated CI runner.

The SSH target is a disposable Debian container with an explicit PVE command fixture;
this verifies installer/authentication behavior without pretending to run a PVE kernel.
"""
import http.cookiejar
import json
import os
import pathlib
import secrets
import sqlite3
import subprocess
import time
import urllib.error
import urllib.request

if os.environ.get('GITHUB_ACTIONS') != 'true' or os.geteuid() != 0:
    raise SystemExit('Only run inside the isolated root CI harness')

def command(args, data=None):
    result = subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=240)
    if result.returncode:
        raise AssertionError('Isolated SSH fixture command failed: ' + args[0])
    return result.stdout.decode().strip()

opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def call(path, data=None, expected=200):
    request = urllib.request.Request('http://127.0.0.1:8087/api/' + path,
        data=None if data is None else json.dumps(data).encode(),
        headers={'Content-Type': 'application/json', 'X-Anker-Request': '1'})
    try:
        response = opener.open(request, timeout=330)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        assert password.encode() not in body, 'One-time SSH credential exposed by Web response'
        assert response.status == expected, (path, response.status, body)
        return json.loads(body)

password = secrets.token_urlsafe(24)
container = None
host_id = None
known = pathlib.Path('/etc/anker/known_hosts')
before = known.read_bytes() if known.exists() else None
mode = known.stat().st_mode & 0o777 if known.exists() else 0o640
call('login', {'name': 'admin', 'password': 'init2026'})
try:
    container = command(['docker', 'run', '-d', '--hostname', 'pve-ssh-fixture', '-p', '127.0.0.1::22', 'debian:13-slim', 'sleep', 'infinity'])
    command(['docker', 'exec', container, 'sh', '-c', 'apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq openssh-server python3 tar passwd && ! command -v sudo'])
    command(['docker', 'exec', '-i', container, 'sh'], b'''set -eu
mkdir -p /run/sshd /etc/pve /etc/ssh/sshd_config.d
printf '%s\n' 'PasswordAuthentication yes' 'PermitRootLogin yes' 'KbdInteractiveAuthentication no' > /etc/ssh/sshd_config.d/00-fixture.conf
printf '%s\n' '#!/bin/sh' 'echo pve-manager/9.0-fixture' > /usr/bin/pveversion
chmod 0755 /usr/bin/pveversion
groupadd -f sudo
useradd --create-home --shell /bin/sh --groups sudo bootstrap
printf '%s\n' '{"paths":["/etc","/usr/local"],"fixture_preserved":true}' >/etc/anker-host.json
chmod 0600 /etc/anker-host.json
ssh-keygen -A >/dev/null
''')
    command(['docker', 'exec', '-i', container, 'chpasswd'], ('root:' + password + '\nbootstrap:' + password + '\n').encode())
    command(['docker', 'exec', '-d', container, '/usr/sbin/sshd', '-D', '-e'])
    port = int(command(['docker', 'port', container, '22']).rsplit(':', 1)[1])
    inspection = None
    for attempt in range(30):
        try:
            inspection = call('hosts/connection/inspect', {'address': '127.0.0.1', 'port': port})
            break
        except AssertionError:
            time.sleep(0.2)
    assert inspection and not inspection['known'] and not inspection['changed'], 'SSH inspection failed'
    actual = command(['docker', 'exec', container, 'ssh-keygen', '-lf', '/etc/ssh/ssh_host_ed25519_key.pub', '-E', 'sha256']).split()[1]
    assert actual == inspection['fingerprint'], 'Inspected SSH identity differs from actual server'
    payload = {'host': {'address': '127.0.0.1', 'ssh_port': port, 'enabled': False, 'group': 'SSH integration'},
        'username': 'root', 'password': password, 'fingerprint': actual, 'confirmed': True}
    failed = dict(payload, password='wrong-one-time-password')
    call('hosts/connection/enroll', failed, expected=400)
    assert call('hosts') == [], 'Failed SSH authentication registered a host'
    assert not known.exists() or known.read_bytes() == before, 'Failed authentication saved trust'
    saved = call('hosts/connection/enroll', payload)
    host_id = saved['id']
    assert saved['name'] == 'pve-ssh-fixture' and saved['inventory']['pve_version'] == '9.0-fixture'
    assert saved['ssh_user'] == 'anker' and saved['restore_ssh_user'] == 'anker-restore'
    assert saved['key_path'] == '/etc/anker/keys/backup' and saved['restore_key_path'] == '/etc/anker/keys/restore'
    assert saved['last_probe'] and not saved.get('probe_error'), 'Initial key-only probes missing'
    import pwd
    for name in ('backup', 'restore'):
        info = pathlib.Path('/etc/anker/keys/' + name).stat()
        assert info.st_uid == pwd.getpwnam('anker').pw_uid and info.st_mode & 0o777 == 0o600, 'Existing service SSH key ownership changed'
    assert json.loads(command(['docker', 'exec', container, 'cat', '/etc/anker-host.json']))['fixture_preserved'], 'Existing host profile changed'
    assert not command(['docker', 'exec', container, 'sh', '-c', 'find /tmp -maxdepth 1 -name "anker-enroll.*" -print']), 'Remote installation package left behind'
    # Existing unrelated authorized_keys and profile must survive a repeat using password-requiring sudo.
    command(['docker', 'exec', container, 'sh', '-c', 'ssh-keygen -q -t ed25519 -N "" -C foreign-fixture -f /tmp/foreign && cat /tmp/foreign.pub >> /var/lib/anker-ssh/.ssh/authorized_keys'])
    original = command(['docker', 'exec', container, 'cat', '/var/lib/anker-ssh/.ssh/authorized_keys']).splitlines()
    metadata = {key: value for key, value in saved.items() if key not in ('inventory', 'last_probe', 'probe_error')}
    repeated = call('hosts/connection/enroll', dict(payload, host=metadata, username='bootstrap'))
    assert repeated['id'] == host_id and len(call('hosts')) == 1, 'Reconfiguration duplicated host'
    after = command(['docker', 'exec', container, 'cat', '/var/lib/anker-ssh/.ssh/authorized_keys']).splitlines()
    assert len(after) == len(original) and set(after) == set(original), 'Repeated installation changed or duplicated managed/foreign keys'
    assert call('hosts/connection/inspect', {'address': '127.0.0.1', 'port': port})['known'], 'Verified identity not persisted'
    # Exercise the real restricted backup role; an arbitrary exec string cannot open a shell.
    ssh = ['ssh', '-p', str(port), '-i', '/etc/anker/keys/backup', '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=/etc/anker/known_hosts', 'anker@127.0.0.1']
    probe = json.loads(command(ssh + ['whoami'], b'{"version":1,"operation":"probe"}\n'))
    assert probe['hostname'] == 'pve-ssh-fixture', 'Forced command permitted arbitrary shell'
    denied = subprocess.run(ssh + ['anker-host'], input=b'{"version":1,"operation":"apply"}\n', stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
    assert denied.returncode != 0, 'Backup key permitted a write request'
    db = sqlite3.connect('/srv/anker/catalog.db')
    try:
        for (row,) in db.execute('SELECT value FROM records'):
            assert password not in (row.decode() if isinstance(row, bytes) else row), 'One-time credential persisted in catalog'
    finally:
        db.close()
    assert known.stat().st_uid == 0 and known.stat().st_mode & 0o777 == 0o640
    print('Real SSH root/sudo onboarding, both restricted roles, retries, existing profile/keys, trust and credential privacy passed.')
finally:
    if host_id:
        request = urllib.request.Request('http://127.0.0.1:8087/api/hosts/' + host_id, method='DELETE', headers={'X-Anker-Request': '1'})
        with opener.open(request, timeout=10) as response:
            assert response.status == 200
    if before is None:
        known.unlink(missing_ok=True)
    else:
        known.write_bytes(before)
        known.chmod(mode)
    if container:
        command(['docker', 'rm', '-f', container])
