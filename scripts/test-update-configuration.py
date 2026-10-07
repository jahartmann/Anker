#!/usr/bin/env python3
"""Real web-to-root update configuration, exclusively on the isolated CI server."""
import json
import os
import pathlib
import subprocess
import time
import urllib.error
import urllib.request
import http.cookiejar

if os.environ.get('GITHUB_ACTIONS') != 'true' or os.geteuid() != 0:
    raise SystemExit('Only run inside the isolated root CI harness')

config = pathlib.Path('/etc/anker/update.json')
before = config.read_bytes()
mode = config.stat().st_mode & 0o777
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

def call(path, data=None, expected=200, authenticated=True):
    request = urllib.request.Request('http://127.0.0.1:8087/api/' + path,
        data=None if data is None else json.dumps(data).encode(),
        headers={'Content-Type': 'application/json', 'X-Anker-Request': '1'})
    try:
        response = opener.open(request, timeout=10) if authenticated else urllib.request.urlopen(request, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        assert response.status == expected, (path, response.status, response.read())
        return json.load(response)

def restart():
    subprocess.run(['systemctl', 'reset-failed', 'anker-updater'], check=True)
    subprocess.run(['systemctl', 'restart', 'anker-updater'], check=True)
    deadline = time.monotonic() + 20
    while True:
        try:
            return call('updates/configuration')
        except AssertionError:
            if time.monotonic() >= deadline:
                raise
            time.sleep(0.2)

call('login', {'name': 'admin', 'password': 'init2026'})
try:
    subprocess.run(['systemctl', 'stop', 'anker-updater'], check=True)
    config.unlink()
    state = restart()
    assert not state['configured'] and state['official'], 'Missing configuration has no shipped official preset'
    call('updates/configuration', authenticated=False, expected=401)
    call('updates/configure', {'confirmed': False}, expected=400)
    assert not config.exists(), 'Unconfirmed source was written'
    result = call('updates/configure', {'repository': 'jahartmann/Anker', 'public_key': '', 'confirmed': True})
    assert result['configured'] and result['official'], result
    saved = config.read_bytes()
    installed = json.loads(saved)
    assert installed['repository'] == 'jahartmann/Anker'
    assert installed['public_key'] == pathlib.Path('public.key').read_text().strip()
    assert config.stat().st_uid == 0 and config.stat().st_mode & 0o077 == 0, 'Update trust is writable/readable by ordinary users'
    assert 'token_file' not in result, 'Root credential path exposed to web'
    call('updates/configure', {'repository': 'jahartmann/Anker', 'public_key': 'invalid', 'confirmed': True}, expected=400)
    assert config.read_bytes() == saved, 'Rejected trust change modified configuration'
    persisted = restart()
    assert persisted['configured'] and persisted['fingerprint'] == result['fingerprint'], 'Web trust configuration lost across helper restart'
    assert call('updates')['configured'], 'Update status not linked to configured helper'
    print('Real Web update source setup, confirmation, authentication, shipped key and restart persistence passed.')
finally:
    subprocess.run(['systemctl', 'stop', 'anker-updater'], check=True)
    config.write_bytes(before)
    config.chmod(mode)
    restart()
