#!/usr/bin/env python3
"""Exercise guided Anker management on a real PTY with an isolated demo service.

Usage: python3 scripts/test-tui-pty.py --binary /path/to/anker
No root, fixed production paths, or external dependencies are required.
"""
import argparse
import errno
import fcntl
import json
import os
import pathlib
import pty
import re
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binary', default=os.environ.get('ANKER_BIN', '/usr/local/bin/anker'))
args = parser.parse_args()
binary = str(pathlib.Path(args.binary).resolve())

with tempfile.TemporaryDirectory(prefix='anker-tui-pty-') as directory:
    data_directory = str(pathlib.Path(directory) / 'data')
    pathlib.Path(data_directory).mkdir()
    daemon_log = open(pathlib.Path(directory) / 'daemon.log', 'w+')
    daemon = subprocess.Popen([binary, '--data', data_directory, '--listen', '127.0.0.1:0', 'demo'], stdout=daemon_log, stderr=daemon_log)
    child = terminal = None
    transcript = bytearray()
    recent = bytearray()

    def status():
        result = subprocess.run([binary, '--data', data_directory, 'status'], text=True, capture_output=True, timeout=10)
        if result.returncode:
            return None
        return json.loads(result.stdout)

    def pump(duration=0.2):
        deadline = time.monotonic() + duration
        while time.monotonic() < deadline:
            ready, _, _ = select.select([terminal], [], [], min(0.05, max(0, deadline - time.monotonic())))
            if not ready:
                continue
            try:
                chunk = os.read(terminal, 65536)
            except OSError as exc:
                if exc.errno == errno.EIO:
                    return
                raise
            transcript.extend(chunk)
            recent.extend(chunk)
            # Answer terminal capability queries without exposing a real terminal.
            if b'\x1b[6n' in chunk:
                os.write(terminal, b'\x1b[1;1R')
            if b'\x1b]11;?' in chunk:
                os.write(terminal, b'\x1b]11;rgb:1010/1010/1010\x07')

    def visible(data):
        data = re.sub(rb'\x1b\][^\x07]*(?:\x07|\x1b\\)', b'', bytes(data))
        return re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', data).decode('utf-8', errors='replace')

    def wait_for(text, timeout=10):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            pump()
            if text in visible(recent):
                return
        raise AssertionError(f'Terminal did not show {text!r}: {visible(recent)[-6000:]}')

    def send(data):
        recent.clear()
        os.write(terminal, data)
        pump(0.25)

    def click(x, y):
        send(f'\x1b[<0;{x + 1};{y + 1}M\x1b[<0;{x + 1};{y + 1}m'.encode())

    def wait_state(predicate, timeout=10):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            current = status()
            if current and predicate(current):
                return current
            if terminal is not None:
                pump(0.2)
            else:
                time.sleep(0.1)
        raise AssertionError('Expected API state was not reached')

    try:
        wait_state(lambda state: len(state['hosts']) >= 6, timeout=30)
        child, terminal = pty.fork()
        if child == 0:
            os.environ['TERM'] = 'xterm-256color'
            os.execv(binary, [binary, '--data', data_directory])
        fcntl.ioctl(terminal, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 80, 0, 0))
        os.kill(child, signal.SIGWINCH)
        wait_for('Hinzufügen')
        send(b'a')
        wait_for('Host hinzufügen')
        send(b'pty-guided-host')
        send(b'\t')
        send(b'192.0.2.42')
        send(b'\x13')  # Ctrl+S: review, no mutation yet.
        wait_for('Host speichern')
        assert not any(h['name'] == 'pty-guided-host' for h in status()['hosts']), 'Host written before approval'
        send(b'y')
        wait_state(lambda state: any(h['name'] == 'pty-guided-host' for h in state['hosts']))
        pump(3.2)  # Let the coalesced status refresh settle.

        # Start a second form and resize while editing; input must survive.
        send(b'a')
        wait_for('Host hinzufügen')
        send(b'preserved-resize')
        fcntl.ioctl(terminal, termios.TIOCSWINSZ, struct.pack('HHHH', 30, 100, 0, 0))
        os.kill(child, signal.SIGWINCH)
        pump(0.5)
        assert 'preserved-resize' in visible(recent), 'Resize discarded the draft'
        send(b'\x1b')
        pump(0.4)

        # Mouse switches tabs and opens users; passwords remain invisible.
        click(70, 2)  # Einstellungen at 100 columns.
        wait_for('Benutzer')
        click(5, 9)
        click(5, 9)  # Open the selected settings item.
        wait_for('Sitzungen')
        send(b'a')
        wait_for('Benutzer hinzufügen')
        send(b'pty-user')
        send(b'\t')
        send(b'\t')
        send(b'\t')
        password = b'unique-pty-password-2026'
        send(password)
        send(b'\t')
        send(password)
        send(b'\x13')
        wait_for('Benutzer anlegen')
        assert password not in transcript, 'Password appeared in terminal output'
        send(b'\x1b')
        pump(0.4)
        send(b'\x1b')
        pump(0.4)
        send(b'\x1b')
        pump(0.4)
        send(b'q')
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            pump()
            finished, code = os.waitpid(child, os.WNOHANG)
            if finished:
                assert os.waitstatus_to_exitcode(code) == 0, 'TUI exited unsuccessfully'
                child = None
                break
        assert child is None, 'TUI did not exit on q'
        print('Guided TUI PTY passed: default TTY start, host approval, resize, mouse, hidden passwords, clean exit.')
    finally:
        if child:
            try:
                os.kill(child, signal.SIGKILL)
                os.waitpid(child, 0)
            except ProcessLookupError:
                pass
        if terminal is not None:
            os.close(terminal)
        daemon.terminate()
        try:
            daemon.wait(timeout=10)
        except subprocess.TimeoutExpired:
            daemon.kill()
            daemon.wait()
        if daemon.returncode not in (0, -signal.SIGTERM):
            daemon_log.seek(0)
            print(daemon_log.read())
        daemon_log.close()
