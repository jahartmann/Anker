#!/usr/bin/env python3
"""Exercise the real setup terminal on an isolated GitHub Linux runner."""
import os
import pathlib
import pty
import select
import signal
import time

if os.environ.get('GITHUB_ACTIONS')!='true' or os.geteuid()!=0:
 raise SystemExit('Only run inside the isolated root CI harness')

def setup(first):
 child,terminal=pty.fork()
 if child==0:os.execv('/usr/local/bin/anker',['anker','setup'])
 steps=[(b'Administratorname',b'\n'),(b'Webzugriff:',b'\n'),(b'Update-Schl',b'\n'),(b'GitHub-Repository',b'\n'),(b'Repository: 1',b'\n'),(b'Einrichtung speichern',b'j\n')]
 if first:steps+=[(b'Administratorpasswort',b'systemd-test-password\n'),(b'Passwort wiederholen',b'systemd-test-password\n')]
 received=b'';deadline=time.monotonic()+90;index=0
 try:
  while time.monotonic()<deadline:
   finished,status=os.waitpid(child,os.WNOHANG)
   if finished:
    if os.waitstatus_to_exitcode(status)!=0 or index!=len(steps):raise RuntimeError('Setup failed: '+received.decode(errors='replace'))
    return
   ready,_,_=select.select([terminal],[],[],0.1)
   if ready:
    try:received+=os.read(terminal,65536)
    except OSError:continue
   if index<len(steps) and steps[index][0] in received:
    # Wait for the complete prompt. The terminal must already have echo disabled.
    if not received.rstrip().endswith(b':'):continue
    os.write(terminal,steps[index][1]);index+=1;received=b''
  raise RuntimeError('Setup timed out: '+received.decode(errors='replace'))
 finally:
  try:os.kill(child,signal.SIGKILL)
  except ProcessLookupError:pass
  try:os.waitpid(child,0)
  except ChildProcessError:pass
  os.close(terminal)

setup(True)
keys={name:pathlib.Path('/etc/anker/keys/'+name).read_bytes() for name in ['backup','restore']}
setup(False)
for name,content in keys.items():
 assert pathlib.Path('/etc/anker/keys/'+name).read_bytes()==content,'Setup replaced an existing SSH key'
print('Terminal setup and repeated setup passed; existing SSH keys preserved.')
