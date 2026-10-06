#!/usr/bin/env python3
"""Exercise the real setup terminal on an isolated GitHub Linux runner."""
import os
import pathlib
import pty
import select
import signal
import ssl
import urllib.request
import json
import shlex
import time

if os.environ.get('GITHUB_ACTIONS')!='true' or os.geteuid()!=0:
 raise SystemExit('Only run inside the isolated root CI harness')

def setup(first,mode="1",tls_choice="1"):
 child,terminal=pty.fork()
 if child==0:os.execv('/usr/local/bin/anker',['anker','setup'])
 steps=[]
 if first:steps+=[(b'Administratorname',b'\n'),(b'Administratorpasswort',b'systemd-test-password\n'),(b'Passwort wiederholen',b'systemd-test-password\n')]
 steps+=[(b'Webzugriff:',mode.encode()+b'\n')]
 if mode=='2':steps+=[(b'Adresse im Browser',b'127.0.0.1\n'),(b'TLS:',tls_choice.encode()+b'\n')]
 steps+=[(b'Einrichtung speichern',b'j\n')]
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

def tls_paths():
 env={line.split('=',1)[0]:shlex.split(line.split('=',1)[1])[0] for line in pathlib.Path('/etc/anker/service.env').read_text().splitlines() if '=' in line}
 return pathlib.Path(env['ANKER_TLS_CERT']),pathlib.Path(env['ANKER_TLS_KEY'])

setup(False,'2','1')
cert,key=tls_paths();original_cert=cert.read_bytes();original_key=key.read_bytes()
context=ssl.create_default_context(cafile=str(cert))
request=urllib.request.Request('https://127.0.0.1:8087/api/login',data=json.dumps({'name':'admin','password':'systemd-test-password'}).encode(),headers={'Content-Type':'application/json','X-Anker-Request':'1'})
with urllib.request.urlopen(request,context=context) as response:
 assert response.status==200
 assert 'Secure' in response.headers['Set-Cookie'],'TLS login cookie is not secure'
setup(False,'2','3')
next_cert,next_key=tls_paths()
assert next_cert==cert and next_key==key,'Repeated setup duplicated TLS files'
assert next_cert.read_bytes()==original_cert and next_key.read_bytes()==original_key,'Repeated setup changed TLS identity'
with urllib.request.urlopen(request,context=context) as response:assert response.status==200
print('Automatic TLS setup, verified HTTPS login and retained certificate identity passed.')
# The updater integration uses its own HTTP loopback login, through the tunnel mode.
setup(False,'1')
