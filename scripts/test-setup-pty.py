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
import subprocess
import re

if os.environ.get('GITHUB_ACTIONS')!='true' or os.geteuid()!=0:
 raise SystemExit('Only run inside the isolated root CI harness')

def restart_updater():
 # Independent restart cases must not consume systemd's production rate limit.
 # Keep the installed unit unchanged, and let any failed restart fail the test.
 subprocess.run(['systemctl','reset-failed','anker-updater'],check=True)
 subprocess.run(['systemctl','restart','anker-updater'],check=True)

def setup(first,mode="1",tls_choice="1",plain=False,reuse=False,cancel=False,expect_error=False,installer=False):
 # This fixture deliberately performs many independent restarts in succession.
 if not first:subprocess.run(['systemctl','reset-failed','anker','anker-updater'],check=True)
 child,terminal=pty.fork()
 if child==0:
  os.environ['TERM']='dumb' if plain else 'xterm-256color'
  if plain:
   os.environ['NO_COLOR']='1';os.environ['ANKER_NO_ANIMATION']='1'
  else:
   os.environ.pop('NO_COLOR',None);os.environ.pop('ANKER_NO_ANIMATION',None)
  if installer:os.execv('./scripts/install-server.sh',['./scripts/install-server.sh'])
  os.execv('/usr/local/bin/anker',['anker','setup'])
 steps=[]
 if first:steps+=[(b'Administratorname',b'\n'),(b'Administratorpasswort',b'init2026\n'),(b'Passwort wiederholen',b'init2026\n')]
 if not first:steps+=[(b'Einrichtung [1]',b'1\n' if reuse else b'2\n')]
 if not reuse:
  steps+=[(b'Webzugriff',mode.encode()+b'\n')]
  if mode=='2':steps+=[(b'Adresse im Browser',b'127.0.0.1\n'),(b'TLS',tls_choice.encode()+b'\n')]
 steps+=[(b'Einrichtung speichern',b'n\n' if cancel else b'j\n')]
 received=b'';transcript=b'';deadline=time.monotonic()+90;index=0
 try:
  while time.monotonic()<deadline:
   finished,status=os.waitpid(child,os.WNOHANG)
   if finished:
    if os.waitstatus_to_exitcode(status)!=(1 if cancel or expect_error else 0) or index!=len(steps):raise RuntimeError('Setup failed: '+received.decode(errors='replace'))
    assert b'init2026' not in transcript,'Password leaked into terminal output'
    if plain:assert b'\x1b' not in transcript,'Plain terminal received escape sequences'
    return transcript
   ready,_,_=select.select([terminal],[],[],0.1)
   if ready:
    try:
     chunk=os.read(terminal,65536);received+=chunk;transcript+=chunk
    except OSError:continue
   visible=re.sub(rb'\x1b\[[0-9;]*[A-Za-z]',b'',received)
   if index<len(steps) and steps[index][0] in visible:
    # Wait for the complete prompt. The terminal must already have echo disabled.
    if not visible.rstrip().endswith(b':'):continue
    os.write(terminal,steps[index][1]);index+=1;received=b''
  raise RuntimeError('Setup timed out: '+received.decode(errors='replace'))
 finally:
  try:os.kill(child,signal.SIGKILL)
  except ProcessLookupError:pass
  try:os.waitpid(child,0)
  except ChildProcessError:pass
  os.close(terminal)

def tls_paths():
 env={line.split('=',1)[0]:shlex.split(line.split('=',1)[1])[0] for line in pathlib.Path('/etc/anker/service.env').read_text().splitlines() if '=' in line}
 return pathlib.Path(env['ANKER_TLS_CERT']),pathlib.Path(env['ANKER_TLS_KEY'])

# Exercise the user's actual installer -> setup path, including inherited umask.
setup(True,'2',installer=True)
first_cert,first_key=tls_paths()
first_identity=(first_cert.read_bytes(),first_key.read_bytes())
first_context=ssl.create_default_context(cafile=str(first_cert))
first_request=urllib.request.Request('https://127.0.0.1:8087/api/login',data=json.dumps({'name':'admin','password':'init2026'}).encode(),headers={'Content-Type':'application/json','X-Anker-Request':'1'})
with urllib.request.urlopen(first_request,context=first_context) as response:assert response.status==200
keys={name:pathlib.Path('/etc/anker/keys/'+name).read_bytes() for name in ['backup','restore']}
# Repair a directory left by the old installer while keeping its TLS identity.
first_cert.parent.chmod(0o700)
setup(False,plain=True,reuse=True)
assert (first_cert.read_bytes(),first_key.read_bytes())==first_identity,'Permission repair replaced TLS identity'
with urllib.request.urlopen(first_request,context=first_context) as response:assert response.status==200
for path in (first_cert,first_key):
 subprocess.run(['runuser','-u','anker','--','test','-r',str(path)],check=True)
assert first_key.stat().st_mode&0o007==0,'Private TLS key became accessible to others'
setup(False)
for name,content in keys.items():
 assert pathlib.Path('/etc/anker/keys/'+name).read_bytes()==content,'Setup replaced an existing SSH key'
print('Terminal setup and repeated setup passed; existing SSH keys preserved.')
print('Installer-to-setup HTTPS and repair of root-only TLS directory passed; private key remains protected.')

# Upgrade a legacy unit, including its disabled-without-update-source condition.
unit=pathlib.Path('/etc/systemd/system/anker-updater.service')
unit.write_text(unit.read_text().replace('[Unit]\n','[Unit]\nConditionPathExists=/etc/anker/update.json\n',1))
update_config=pathlib.Path('/etc/anker/update.json');saved_update=update_config.with_suffix('.ci-save')
release_key=pathlib.Path('/etc/anker/release-public.key');saved_key=release_key.with_suffix('.ci-save')
subprocess.run(['systemctl','stop','anker-updater'],check=True)
update_config.rename(saved_update);release_key.rename(saved_key)
try:
 subprocess.run(['systemctl','daemon-reload'],check=True)
 setup(False)
 assert 'ConditionPathExists=/etc/anker/update.json' not in unit.read_text(),'Legacy unit not migrated'
 assert subprocess.run(['systemctl','is-active','--quiet','anker-updater']).returncode==0,'Helper without release config did not start'
 print('Legacy updater unit migration and helper startup without release configuration passed.')
finally:
 saved_update.rename(update_config);saved_key.rename(release_key)
 restart_updater()

setup(False,'2','1')
cert,key=tls_paths();original_cert=cert.read_bytes();original_key=key.read_bytes()
context=ssl.create_default_context(cafile=str(cert))
request=urllib.request.Request('https://127.0.0.1:8087/api/login',data=json.dumps({'name':'admin','password':'init2026'}).encode(),headers={'Content-Type':'application/json','X-Anker-Request':'1'})
with urllib.request.urlopen(request,context=context) as response:
 assert response.status==200
 assert 'Secure' in response.headers['Set-Cookie'],'TLS login cookie is not secure'
setup(False,'2','3')
next_cert,next_key=tls_paths()
assert next_cert==cert and next_key==key,'Repeated setup duplicated TLS files'
assert next_cert.read_bytes()==original_cert and next_key.read_bytes()==original_key,'Repeated setup changed TLS identity'
with urllib.request.urlopen(request,context=context) as response:assert response.status==200
# Reuse must skip the connection questions and retain custom settings.
env_path=pathlib.Path('/etc/anker/service.env')
original_env=env_path.read_bytes()
policy_path=pathlib.Path('/etc/anker/tls-renewal.json')
original_policy=policy_path.read_bytes()
transcript=setup(False,reuse=True)
assert b'Adresse im Browser' not in transcript,'Reuse asked to configure the connection again'
assert tls_paths()==(cert,key),'Reuse replaced TLS paths'
assert cert.read_bytes()==original_cert and key.read_bytes()==original_key,'Reuse replaced TLS identity'
assert policy_path.read_bytes()==original_policy,'Reuse changed renewal policy'
# Cancellation before saving must leave services and configuration alone.
main_pid=subprocess.check_output(['systemctl','show','anker','--property=MainPID','--value']).strip()
setup(False,'1',cancel=True)
assert env_path.read_bytes()==original_env,'Cancelled setup changed connection'
assert subprocess.check_output(['systemctl','show','anker','--property=MainPID','--value']).strip()==main_pid,'Cancelled setup restarted service'
# Failure after the web service has started must still restore all settings.
dropin=pathlib.Path('/etc/systemd/system/anker-updater.service.d')
dropin.mkdir(exist_ok=True)
failure=dropin/'99-ci-setup-failure.conf'
failure.write_text('[Service]\nExecStart=\nExecStart=/bin/false\nRestart=no\n')
try:
 subprocess.run(['systemctl','daemon-reload'],check=True)
 setup(False,'1',expect_error=True)
 assert env_path.read_bytes()==original_env,'Helper startup failure left new web configuration active'
 assert policy_path.read_bytes()==original_policy,'Helper startup failure changed TLS policy'
 assert not pathlib.Path('/etc/anker/setup-pending.json').exists(),'Error rollback left a journal'
finally:
 failure.unlink()
 subprocess.run(['systemctl','daemon-reload'],check=True)
 restart_updater()
with urllib.request.urlopen(request,context=context) as response:assert response.status==200
print('System service startup failure restored previous HTTPS configuration.')
# Durable journal fixture reproduces a killed process with partially written files.
import base64
saved=[]
for name in ['service.env','tls-renewal.json','update.json','setup-complete']:
 path=pathlib.Path('/etc/anker')/name
 if path.exists():
  info=path.stat();saved.append({'Name':name,'Exists':True,'Data':base64.b64encode(path.read_bytes()).decode(),'Mode':info.st_mode&0o777,'UID':info.st_uid,'GID':info.st_gid})
 else:saved.append({'Name':name,'Exists':False,'Data':None,'Mode':0,'UID':0,'GID':0})
pending=pathlib.Path('/etc/anker/setup-pending.json')
unused=pathlib.Path('/etc/anker/tls/server-interrupted.crt')
pending.write_text(json.dumps({'Version':1,'Files':saved,'Created':[str(unused)],'MainRunning':True,'UpdaterRunning':True}));pending.chmod(0o600)
subprocess.run(['systemctl','stop','anker-updater','anker'],check=True)
env_path.write_text('ANKER_LISTEN=127.0.0.1:9191\n')
policy_path.write_text('{}\n');unused.write_text('partial certificate')
transcript=setup(False,reuse=True)
assert 'Unterbrochene Einrichtung erkannt'.encode() in transcript,'Interrupted setup not recognized'
assert env_path.read_bytes()==original_env,'Interrupted configuration not recovered'
assert not pending.exists() and not unused.exists(),'Pending configuration not cleaned up'
assert cert.read_bytes()==original_cert and key.read_bytes()==original_key,'Recovery changed existing TLS identity'
for name,content in keys.items():assert pathlib.Path('/etc/anker/keys/'+name).read_bytes()==content,'Recovery replaced SSH key'
with urllib.request.urlopen(request,context=context) as response:assert response.status==200
print('Resume, cancellation and interrupted configuration recovery passed.')
print('Automatic TLS setup, verified HTTPS login and retained certificate identity passed.')

def tls_cli(*args):
 return json.loads(subprocess.check_output(['/usr/local/bin/anker','tls',*args]))

# Certificate management must also work before a signed update source exists.
update_config=pathlib.Path('/etc/anker/update.json')
saved_update=update_config.with_suffix('.ci-save')
update_config.rename(saved_update)
try:
 restart_updater()
 deadline=time.monotonic()+20
 while True:
  try:tls_status=tls_cli('status');break
  except subprocess.CalledProcessError:
   if time.monotonic()>deadline:raise
   time.sleep(0.2)
 main_pid=subprocess.check_output(['systemctl','show','anker','--property=MainPID','--value']).strip()
 renewed=tls_cli('renew')
 assert renewed['fingerprint']!=tls_status['fingerprint'],'Certificate not renewed'
 assert key.read_bytes()==original_key,'Renewal replaced private TLS key'
 assert subprocess.check_output(['systemctl','show','anker','--property=MainPID','--value']).strip()==main_pid,'TLS renewal restarted main service'
 # Trust the independently read new certificate and make a real TLS request.
 context=ssl.create_default_context(cafile=str(cert))
 with urllib.request.urlopen(request,context=context) as response:assert response.status==200
 tls_cli('auto','off','--days','14')
 restart_updater()
 deadline=time.monotonic()+20
 while True:
  try:policy=tls_cli('status');break
  except subprocess.CalledProcessError:
   if time.monotonic()>deadline:raise
   time.sleep(0.2)
 assert not policy['automatic'] and policy['renew_before_days']==14,'Renewal policy lost across restart'
 # Create a short-lived fixture with the same key and SANs, then prove that
 # the background startup check actually renews it without a manual command.
 staged=cert.with_suffix('.ci-short')
 subprocess.run(['openssl','x509','-in',str(cert),'-signkey',str(key),'-days','10','-out',str(staged)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 os.chown(staged,cert.stat().st_uid,cert.stat().st_gid);staged.chmod(0o640);staged.replace(cert)
 tls_cli('auto','on','--days','30')
 restart_updater()
 deadline=time.monotonic()+20
 while True:
  try:
   status=tls_cli('status')
   if status['days_remaining']>360:break
  except subprocess.CalledProcessError:pass
  if time.monotonic()>deadline:raise RuntimeError('Background renewal did not complete')
  time.sleep(0.2)
 assert key.read_bytes()==original_key,'Automatic renewal replaced TLS key'
 assert subprocess.check_output(['systemctl','show','anker','--property=MainPID','--value']).strip()==main_pid,'Automatic TLS renewal restarted main service'
 context=ssl.create_default_context(cafile=str(cert))
 with urllib.request.urlopen(request,context=context) as response:assert response.status==200
 print('Manual and automatic TLS renewal without service restart or release configuration passed; policy survived helper restart.')
finally:
 saved_update.rename(update_config)
 restart_updater()
# The updater integration uses its own HTTP loopback login, through the tunnel mode.
setup(False,'1')
