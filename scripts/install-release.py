#!/usr/bin/env python3
"""Download a signed release, verify it, then open the server setup."""
import argparse
import base64
import getpass
import grp
import hashlib
import json
import os
import pathlib
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import urllib.parse
import urllib.request

MAX_DOWNLOAD=256*1024*1024
REPOSITORY=re.compile(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\Z')

class SafeRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,req,fp,code,msg,headers,newurl):
  parsed=urllib.parse.urlparse(newurl)
  if parsed.scheme!='https' or parsed.username or parsed.password:
   raise ValueError('Download-Weiterleitung muss HTTPS verwenden')
  redirected=super().redirect_request(req,fp,code,msg,headers,newurl)
  if redirected and parsed.netloc!='api.github.com':redirected.remove_header('Authorization')
  return redirected

def download(url,limit,token='',asset=False):
 parsed=urllib.parse.urlparse(url)
 if parsed.scheme!='https' or parsed.netloc!='api.github.com':raise ValueError('Ungültige GitHub-API-Adresse')
 headers={'User-Agent':'Anker-installer','Accept':'application/octet-stream' if asset else 'application/vnd.github+json'}
 if token:headers['Authorization']='Bearer '+token
 request=urllib.request.Request(url,headers=headers)
 opener=urllib.request.build_opener(SafeRedirect())
 with opener.open(request,timeout=60) as response:
  chunks=[];size=0
  while True:
   chunk=response.read(min(1024*1024,limit+1-size))
   if not chunk:break
   size+=len(chunk)
   if size>limit:raise ValueError('Download überschreitet die erlaubte Größe')
   chunks.append(chunk)
  return b''.join(chunks)

def openssl(*arguments):
 try:return subprocess.run(['openssl',*map(str,arguments)],check=True,capture_output=True).stdout
 except subprocess.CalledProcessError as error:raise ValueError('OpenSSL-Prüfung fehlgeschlagen; Signatur/Schlüssel und OpenSSL 3 prüfen') from error

def public_key(public):
 der=openssl('pkey','-pubin','-in',public,'-outform','DER')
 prefix=bytes.fromhex('302a300506032b6570032100')
 if len(der)!=44 or not der.startswith(prefix):raise ValueError('Ein öffentlicher Ed25519-Schlüssel wird benötigt')
 return base64.b64encode(der[12:]).decode()

def verify_package(public,raw,signature,package,tag,arch):
 with tempfile.TemporaryDirectory(prefix='anker-signature-') as temp:
  root=pathlib.Path(temp);(root/'manifest').write_bytes(raw)
  try:decoded=base64.b64decode(signature.strip(),validate=True)
  except ValueError as error:raise ValueError('Signaturformat ungültig') from error
  if len(decoded)!=64:raise ValueError('Signaturlänge ungültig')
  (root/'signature').write_bytes(decoded)
  openssl('pkeyutl','-verify','-pubin','-inkey',public,'-rawin','-in',root/'manifest','-sigfile',root/'signature')
 manifest=json.loads(raw)
 if manifest.get('format')!=1 or manifest.get('version')!=tag or not re.fullmatch(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)',tag):
  raise ValueError('Release-Version oder Format ungültig')
 assets=[item for item in manifest['assets'] if item['name']==package.name]
 if len(assets)!=1:raise ValueError('Paket fehlt im signierten Manifest')
 asset=assets[0]
 if asset.get('kind')!='bundle' or asset.get('os')!='linux' or asset.get('arch')!=arch:raise ValueError('Paketplattform stimmt nicht')
 if not 0<asset['size']<=MAX_DOWNLOAD or package.stat().st_size!=asset['size']:raise ValueError('Paketgröße stimmt nicht')
 digest=hashlib.sha256()
 with package.open('rb') as stream:
  for chunk in iter(lambda:stream.read(1024*1024),b''):digest.update(chunk)
 if digest.hexdigest()!=asset['sha256']:raise ValueError('Paketprüfsumme stimmt nicht')

def extract_package(package,destination):
 destination=pathlib.Path(destination);destination.mkdir(mode=0o700)
 with tarfile.open(package,'r:gz') as archive:
  members=archive.getmembers();total=0
  if len(members)>10000:raise ValueError('Zu viele Dateien im Paket')
  for member in members:
   name=pathlib.PurePosixPath(member.name)
   if name.is_absolute() or '..' in name.parts or not (member.isdir() or member.isreg()):raise ValueError('Unsicherer Archivpfad oder Link')
   total+=member.size
   if member.size<0 or total>512*1024*1024:raise ValueError('Entpacktes Paket zu groß')
  for member in members:
   target=destination/pathlib.PurePosixPath(member.name)
   if member.isdir():target.mkdir(parents=True,exist_ok=True);continue
   target.parent.mkdir(parents=True,exist_ok=True)
   with archive.extractfile(member) as source,target.open('xb') as output:shutil.copyfileobj(source,output)
   target.chmod(member.mode&0o755)

def write_config(path,content,mode,gid=0):
 path=pathlib.Path(path)
 with tempfile.NamedTemporaryFile(dir=path.parent,prefix='.anker-bootstrap-',delete=False) as stream:
  staged=pathlib.Path(stream.name)
  try:
   stream.write(content);stream.flush();os.fchmod(stream.fileno(),mode);os.fchown(stream.fileno(),0,gid);os.fsync(stream.fileno())
   os.replace(staged,path)
   fd=os.open(path.parent,os.O_RDONLY)
   try:os.fsync(fd)
   finally:os.close(fd)
  finally:staged.unlink(missing_ok=True)

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--public-key',required=True,help='Unabhängig geprüfte public.pem')
 parser.add_argument('--repo',default='jahartmann/Anker')
 parser.add_argument('--private',action='store_true',help='GitHub-Lese-Token verdeckt abfragen')
 parser.add_argument('--token-file',help='Vorhandene geschützte Datei mit GitHub-Lese-Token')
 args=parser.parse_args()
 if platform.system()!='Linux' or os.geteuid()!=0:raise ValueError('Als root auf dem zentralen Linux-/systemd-Server ausführen')
 if not pathlib.Path('/run/systemd/system').is_dir():raise ValueError('Laufendes systemd wird benötigt')
 if not REPOSITORY.fullmatch(args.repo) or '..' in args.repo:raise ValueError('Repository als owner/repo angeben')
 if pathlib.Path('/usr/local/bin/anker').exists() and not pathlib.Path('/etc/anker/install-pending').is_file():raise ValueError('Anker ist bereits installiert: sudo anker setup oder sudo anker update install verwenden')
 if args.private and args.token_file:raise ValueError('Entweder --private oder --token-file verwenden')
 if not os.isatty(0):raise ValueError('Die Ersteinrichtung benötigt ein Terminal')
 public=pathlib.Path(args.public_key).resolve();raw_key=public_key(public)
 token=''
 if args.token_file:
  info=pathlib.Path(args.token_file).lstat()
  if not pathlib.Path(args.token_file).is_file() or pathlib.Path(args.token_file).is_symlink() or info.st_mode&0o077 or info.st_uid!=0 or info.st_size>4096:raise ValueError('Token-Datei muss root gehören und Rechte 0600 haben')
  token=pathlib.Path(args.token_file).read_text().strip()
 elif args.private:token=getpass.getpass('GitHub-Lese-Token (Contents: read-only): ').strip()
 if (args.private or args.token_file) and not token:raise ValueError('GitHub-Token fehlt')
 arch={'x86_64':'amd64','aarch64':'arm64','arm64':'arm64'}.get(platform.machine())
 if not arch:raise ValueError('Nur amd64 und arm64 werden unterstützt')
 latest=json.loads(download('https://api.github.com/repos/'+args.repo+'/releases/latest',1024*1024,token))
 if latest.get('draft') or latest.get('prerelease'):raise ValueError('Stabiler veröffentlichter Release benötigt')
 def asset(name,limit):
  matches=[entry for entry in latest['assets'] if entry['name']==name]
  if len(matches)!=1 or not isinstance(matches[0]['id'],int):raise ValueError('Release-Datei fehlt: '+name)
  return download('https://api.github.com/repos/'+args.repo+'/releases/assets/'+str(matches[0]['id']),limit,token,True)
 with tempfile.TemporaryDirectory(prefix='anker-install-') as temp:
  root=pathlib.Path(temp);package=root/('anker-linux-'+arch+'.tar.gz')
  raw=asset('release.json',1024*1024);signature=asset('release.json.sig',1024)
  package.write_bytes(asset(package.name,MAX_DOWNLOAD))
  verify_package(public,raw,signature,package,latest['tag_name'],arch)
  print('Signatur und Paket geprüft:',latest['tag_name'],arch,flush=True)
  destination=root/'package';extract_package(package,destination)
  subprocess.run(['sh',str(destination/'scripts/install-server.sh'),'--no-setup'],check=True)
  cfg={'repository':args.repo,'public_key':raw_key}
  if token:
   write_config('/etc/anker/github-token',(token+'\n').encode(),0o600)
   cfg['token_file']='/etc/anker/github-token'
  group=grp.getgrnam('anker').gr_gid
  write_config('/etc/anker/release-public.key',(raw_key+'\n').encode(),0o640,group)
  write_config('/etc/anker/update.json',json.dumps(cfg,indent=2).encode()+b'\n',0o640,group)
  subprocess.run(['/usr/local/bin/anker','setup'],check=True)

if __name__=='__main__':
 try:main()
 except (ValueError,OSError,subprocess.CalledProcessError) as error:
  raise SystemExit('Anker-Installation: '+str(error))
