#!/usr/bin/env python3
"""Anker host protocol v1. Install root-owned; stdin JSON, stdout JSON or tar.
No daemon, no user-supplied commands, no shell interpolation.
"""
import base64, hashlib, json, os, pathlib, shutil, socket, sqlite3, stat, subprocess, sys, tarfile, tempfile, time

MAX_FILE = 64 * 1024 * 1024
MAX_REQUEST = 32 * 1024 * 1024

def digest(data): return hashlib.sha256(data).hexdigest()
def command(args):
 try:
  p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=20,check=False)
  return p.stdout.decode('utf-8','replace')[:8*1024*1024] if p.returncode==0 else ''
 except (OSError,subprocess.TimeoutExpired): return ''
def asjson(text,default):
 try: return json.loads(text)
 except (ValueError,TypeError): return default

def probe(root=pathlib.Path('/'),test_mode=False):
 if test_mode:
  p=root/'inventory.json'
  return json.loads(p.read_text()) if p.exists() else {'hostname':'test','pve_version':'8.4','interfaces':[],'disks':[],'details':{},'cluster_id':'','quorate':False}
 version=command(['pveversion']).strip();version=version.split('/')[1] if '/' in version else version
 interfaces=asjson(command(['ip','-j','link','show']),[])
 net=[]
 for i in interfaces:
  name=i.get('ifname','');pci=''
  try:pci=str((pathlib.Path('/sys/class/net')/name/'device').resolve().name)
  except OSError:pass
  net.append({'name':name,'mac':i.get('address',''),'pci':pci})
 blk=asjson(command(['lsblk','-J','-b','-o','NAME,TYPE,SIZE,UUID,MOUNTPOINT,SERIAL,WWN']),{})
 disks=[]
 def flatten(items):
  for d in items:
   disks.append({'name':d.get('name',''),'id':d.get('wwn') or d.get('serial') or d.get('uuid') or d.get('name',''),'size':int(d.get('size') or 0),'uuid':d.get('uuid') or '', 'mount':d.get('mountpoint') or ''})
   flatten(d.get('children',[]))
 flatten(blk.get('blockdevices',[]))
 cluster=command(['pvecm','status']);cluster_id='';quorate=False
 for line in cluster.splitlines():
  if line.startswith('Name:'):cluster_id=line.split(':',1)[1].strip()
  if line.startswith('Quorate:'):quorate=line.split(':',1)[1].strip().lower()=='yes'
 details={
  'addresses':asjson(command(['ip','-j','addr','show']),[]),
  'routes':asjson(command(['ip','-j','route','show']),[]),
  'packages':command(['dpkg-query','-W','-f=${binary:Package} ${Version}\n']),
  'manual_packages':command(['apt-mark','showmanual']),
  'storage':command(['pvesm','status']), 'zfs':command(['zpool','list','-Hp']),
  'lvm':command(['vgs','--reportformat','json']),
  'guests':command(['qm','list']), 'containers':command(['pct','list']),
  'cluster':cluster, 'ceph':command(['ceph','status','--format','json']),
  'pci':command(['lspci','-nn']), 'boot':command(['proxmox-boot-tool','status']),
 }
 # Capture content preconditions; unreadable targets are never treated as missing.
 hashes={}
 for base in ('etc','usr/local'):
  start=root/base
  if not start.exists():continue
  for folder,dirs,files in os.walk(start,followlinks=False):
   dirs[:]=[x for x in dirs if not (pathlib.Path(folder)/x).is_symlink()]
   for name in files:
    p=pathlib.Path(folder)/name;rel=p.relative_to(root).as_posix()
    try:
     if p.is_symlink():hashes[rel]='symlink:'+os.readlink(p)
     elif p.stat().st_size<=MAX_FILE:hashes[rel]=digest(p.read_bytes())
     else:hashes[rel]='too-large'
    except OSError:hashes[rel]='unreadable'
 details['file_hashes']=hashes
 try:details['boot_id']=(root/'proc/sys/kernel/random/boot_id').read_text().strip()
 except OSError:details['boot_id']=''
 try:debian=(root/'etc/debian_version').read_text().strip()
 except OSError:debian=''
 return {'hostname':socket.gethostname(),'pve_version':version,'debian':debian,'kernel':os.uname().release,'boot_mode':'UEFI' if (root/'sys/firmware/efi').exists() else 'BIOS','cluster_id':cluster_id,'quorate':quorate,'interfaces':net,'disks':disks,'details':details,'captured_at':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())}

def relative(path):
 if not path or path.startswith('/') or '\\' in path or '\x00' in path or any(x in ('','..','.') for x in path.split('/')):raise ValueError('invalid path')
 return pathlib.PurePosixPath(path)
def confined(root,path):
 relative(path);dest=root
 for part in path.split('/'):
  dest=dest/part
  if dest.is_symlink():raise ValueError('symlink parent or destination')
 return dest

def entry_meta(p,rel):
 st=p.lstat();mode=stat.S_IMODE(st.st_mode)
 e={'path':rel,'mode':mode,'uid':st.st_uid,'gid':st.st_gid,'mtime':int(st.st_mtime),'size':st.st_size,'xattrs':{}}
 if stat.S_ISLNK(st.st_mode):e.update(type='symlink',link=os.readlink(p))
 elif stat.S_ISDIR(st.st_mode):e['type']='directory'
 elif stat.S_ISREG(st.st_mode):e['type']='file'
 else:return None
 try:
  for name in os.listxattr(p,follow_symlinks=False):e['xattrs'][name]=base64.b64encode(os.getxattr(p,name,follow_symlinks=False)).decode()
 except (OSError,AttributeError):pass
 return e

def snapshot_db(src,out):
 source=sqlite3.connect('file:'+str(src)+'?mode=ro',uri=True,timeout=5)
 dest=sqlite3.connect(out)
 try:
  source.backup(dest,pages=128,sleep=0.05)
  if dest.execute('PRAGMA integrity_check').fetchone()[0]!='ok':raise ValueError('SQLite integrity failure')
 finally:dest.close();source.close()
 os.chmod(out,0o600)

def decode_db(db,out):
 conn=sqlite3.connect('file:'+str(db)+'?mode=ro',uri=True)
 try:rows=conn.execute('SELECT inode,parent,mtime,type,name,data FROM tree').fetchall()
 finally:conn.close()
 nodes={int(r[0]):r for r in rows};paths={0:''}
 def path_for(inode,visited=None):
  if inode in paths:return paths[inode]
  visited=set() if visited is None else visited
  if inode in visited or inode not in nodes:raise ValueError('invalid pmxcfs tree')
  visited.add(inode);r=nodes[inode];name=r[4]
  if not isinstance(name,str) or '/' in name or name in ('','..','.'):raise ValueError('invalid pmxcfs name')
  parent=path_for(int(r[1]),visited);p=parent+'/'+name if parent else name;relative(p);paths[inode]=p;return p
 entries=[]
 for inode,parent,mtime,kind,name,data in rows:
  if int(inode)==0:continue
  rel='etc/pve/'+path_for(int(inode));p=confined(out/'files',rel)
  mode=0o600 if '/priv/' in rel else 0o640
  e={'path':rel,'mode':mode,'uid':0,'gid':33,'mtime':int(mtime or 0),'size':0,'xattrs':{}}
  if kind==4:p.mkdir(parents=True,exist_ok=True);e['type']='directory';e['mode']=0o750
  elif kind==8:
   p.parent.mkdir(parents=True,exist_ok=True);content=bytes(data or b'');p.write_bytes(content);os.chmod(p,0o600);e['type']='file';e['size']=len(content)
  else:raise ValueError('unsupported pmxcfs entry type')
  entries.append(e)
 return entries

def collect(root,out,paths,test_mode=False):
 root=pathlib.Path(root);out=pathlib.Path(out);(out/'files').mkdir(exist_ok=True)
 entries=[];warnings=[];seen=set()
 def walk(p):
  rel=p.relative_to(root).as_posix()
  if rel=='etc/pve' or rel.startswith('etc/pve/'):return
  if rel in seen:return
  seen.add(rel)
  try:
   e=entry_meta(p,rel)
   if not e:warnings.append('Unsupported file type: /'+rel);return
   dest=confined(out/'files',rel)
   if e['type']=='directory':
    dest.mkdir(parents=True,exist_ok=True)
    entries.append(e)
    for child in sorted(p.iterdir()):walk(child)
   elif e['type']=='symlink':dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text(e['link']);entries.append(e)
   else:
    if e['size']>MAX_FILE:raise ValueError('file exceeds 64 MiB limit')
    data=None
    for attempt in range(3):
     before=p.stat();data=p.read_bytes();after=p.stat()
     if (before.st_mtime_ns,before.st_size,before.st_ino)==(after.st_mtime_ns,after.st_size,after.st_ino):break
    else:raise ValueError('file changing during capture')
    dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(data);os.chmod(dest,0o600);entries.append(e)
  except (OSError,ValueError) as exc:warnings.append('Cannot capture /'+rel+': '+str(exc))
 for source in paths:
  p=root/source.lstrip('/')
  if p.exists() or p.is_symlink():walk(p)
  else:warnings.append('Required path missing: '+source)
 db=root/'var/lib/pve-cluster/config.db'
 try:
  (out/'recovery').mkdir(exist_ok=True)
  snapshot_db(db,out/'recovery/config.db')
  entries.extend(decode_db(out/'recovery/config.db',out))
 except (sqlite3.Error,OSError,ValueError) as exc:warnings.append('config.db capture failed: '+str(exc))
 return {'inventory':probe(root,test_mode),'entries':entries,'warnings':warnings}

def apply_files(root,items,test_mode=False):
 root=pathlib.Path(root);prepared=[]
 for e in items:
  p=confined(root,e['path'])
  if not (e['path'].startswith('etc/') or e['path'].startswith('usr/local/')):raise ValueError('restore path not allowed')
  if e['path'].startswith('etc/pve/') or e['path'] in ('etc/shadow','etc/passwd','etc/group','etc/gshadow','etc/fstab','etc/machine-id'):raise ValueError('protected restore requires manual procedure')
  old=p.read_bytes() if p.exists() else None
  if e.get('before_sha')!=('missing' if old is None else digest(old)):raise ValueError('target file changed: '+e['path'])
  content=base64.b64decode(e['content'],validate=True)
  if len(content)>MAX_FILE:raise ValueError('restore file too large')
  if e.get('type','file')!='file':raise ValueError('symlink restore needs manual metadata procedure')
  prepared.append((p,e,old,content))
 rollback=root/'var/lib/anker-host/rollback'/('%d-%s'%(time.time_ns(),os.getpid()));rollback.mkdir(parents=True,mode=0o700)
 before=[];applied=[]
 for p,e,old,content in prepared:
  rel=e['path'];before.append(dict(entry_meta(p,rel),existed=True) if old is not None else {'path':rel,'existed':False})
  if old is not None:
   b=rollback/rel;b.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,b,follow_symlinks=False);os.chmod(b,0o600)
 (rollback/'before.json').write_text(json.dumps(before,indent=2))
 try:
  for p,e,old,content in prepared:
   p.parent.mkdir(parents=True,exist_ok=True)
   fd,tmp=tempfile.mkstemp(prefix='.anker-',dir=p.parent)
   try:
    with os.fdopen(fd,'wb') as f:
     f.write(content);f.flush();os.fsync(f.fileno());os.fchmod(f.fileno(),int(e.get('mode',0o600)) & 0o777)
     if not test_mode:os.fchown(f.fileno(),int(e.get('uid',0)),int(e.get('gid',0)))
    os.replace(tmp,p);applied.append(e['path'])
   finally:
    if os.path.exists(tmp):os.unlink(tmp)
  return {'applied':applied,'rollback_path':str(rollback),'checks':['file hashes verified; service and reboot checks require operator'],'reboot_verified':False}
 except Exception as exc:
  (rollback/'failure.json').write_text(json.dumps({'applied':applied,'error':str(exc)}))
  raise

def main():
 if os.geteuid()!=0 or sys.platform!='linux':raise ValueError('host helper requires root on Linux')
 if len(sys.argv)>1:raise ValueError('no arguments allowed')
 data=sys.stdin.buffer.readline(MAX_REQUEST+1)
 if len(data)>MAX_REQUEST:raise ValueError('request too large')
 request=json.loads(data);op=request.get('operation')
 if request.get('version')!=1:raise ValueError('unsupported protocol version')
 root=pathlib.Path('/')
 if op=='probe':print(json.dumps(probe()))
 elif op=='collect':
  configpath=pathlib.Path('/etc/anker-host.json')
  cfg=json.loads(configpath.read_text()) if configpath.exists() else {'paths':['/etc','/usr/local']}
  if configpath.exists() and (configpath.stat().st_uid!=0 or stat.S_IMODE(configpath.stat().st_mode)&0o022):raise ValueError('host profile must be root-owned and not writable by others')
  paths=cfg.get('paths',['/etc','/usr/local'])
  for p in paths:
   if not isinstance(p,str) or not p.startswith('/') or '..' in p or p in ('/','/proc','/sys','/dev','/var/lib'):raise ValueError('invalid host profile path')
  with tempfile.TemporaryDirectory(prefix='anker-host-') as d:
   out=pathlib.Path(d);c=collect(root,out,paths);(out/'collection.json').write_text(json.dumps(c))
   with tarfile.open(fileobj=sys.stdout.buffer,mode='w|') as t:
    for p in sorted(out.rglob('*')):t.add(p,arcname=p.relative_to(out).as_posix(),recursive=False)
 elif op=='apply':
  if not request.get('confirm') or request.get('confirm')!=request.get('plan_id'):raise ValueError('explicit plan confirmation required')
  expected=dict(request['expected_inventory']);current=probe()
  for x in ('captured_at','fingerprint'):expected.pop(x,None);current.pop(x,None)
  stable=('file_hashes','boot_id','addresses','routes','packages','manual_packages','pci','boot')
  for inv in (expected,current):inv['details']={k:v for k,v in inv.get('details',{}).items() if k in stable}
  def normalize(v):
   if isinstance(v,dict):return {k:normalize(x) for k,x in v.items() if x not in ('',None,[],{})}
   if isinstance(v,list):return [normalize(x) for x in v]
   return v
  if normalize(expected)!=normalize(current):raise ValueError('target inventory changed')
  print(json.dumps(apply_files(root,request['files'])))
 else:raise ValueError('unknown operation')
if __name__=='__main__':
 try:main()
 except Exception as exc:
  print('Anker host operation failed: '+str(exc),file=sys.stderr);sys.exit(1)
