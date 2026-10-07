#!/usr/bin/env python3
"""Anker host envelope v1, hardened restore protocol v2. Install root-owned; stdin JSON, stdout JSON or tar.
No daemon, no user-supplied commands, no shell interpolation.
"""
import contextlib, fcntl, pwd, grp, re, errno, base64, hashlib, json, os, pathlib, shutil, socket, sqlite3, stat, subprocess, sys, tarfile, tempfile, time

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
 root=pathlib.Path(root)
 if test_mode:
  p=root/'inventory.json'
  return json.loads(p.read_text()) if p.exists() else {'hostname':'test','pve_version':'8.4','interfaces':[],'disks':[],'details':{},'cluster_id':'','quorate':False}
 results={}
 def run(key,args,optional=False,parse=False):
  try:
   p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=20,check=False)
   if p.returncode!=0:raise OSError('exit %d: %s'%(p.returncode,p.stderr.decode('utf-8','replace')[:2048]))
   if len(p.stdout)>8*1024*1024:raise ValueError('command output truncated')
   raw=p.stdout.decode('utf-8','replace');value=json.loads(raw) if parse else raw
   if parse and ((key=='disks' and not isinstance(value,dict)) or (key!='disks' and not isinstance(value,list))):raise ValueError('unexpected command JSON shape')
   results[key]={'state':'ok'};return value
  except FileNotFoundError as exc:
   results[key]={'state':'not_applicable' if optional else 'failed','error':str(exc)}
  except (OSError,ValueError,subprocess.TimeoutExpired) as exc:results[key]={'state':'failed','error':str(exc)}
  return {} if key=='disks' else [] if parse else ''
 version=run('pve_version',['pveversion']).strip();version=version.split('/')[1] if '/' in version else version
 interfaces=run('interfaces',['ip','-j','link','show'],parse=True);net=[]
 for i in interfaces:
  name=i.get('ifname','');device=root/'sys/class/net'/name/'device';physical=device.exists();pci=''
  if physical:
   try:pci=device.resolve().name
   except OSError as exc:results['interfaces']={'state':'failed','error':str(exc)}
  net.append({'name':name,'mac':i.get('address',''),'pci':pci,'type':i.get('linkinfo',{}).get('info_kind') or ('physical' if physical else 'virtual'),'physical':physical})
 blk=run('disks',['lsblk','-J','-b','-o','NAME,TYPE,SIZE,UUID,MOUNTPOINT,SERIAL,WWN,FSTYPE'],parse=True);disks=[]
 def flatten(items,parent=''):
  for d in items:
   disks.append({'name':d.get('name',''),'id':d.get('wwn') or d.get('serial') or d.get('uuid') or d.get('name',''),'size':int(d.get('size') or 0),'uuid':d.get('uuid') or '', 'mount':d.get('mountpoint') or '', 'type':d.get('type') or '', 'parent':parent,'filesystem':d.get('fstype') or ''})
   flatten(d.get('children',[]),d.get('name',''))
 try:flatten(blk.get('blockdevices',[]))
 except (ValueError,TypeError,AttributeError) as exc:results['disks']={'state':'failed','error':str(exc)}
 if (root/'etc/pve/corosync.conf').exists():cluster=run('cluster',['pvecm','status'])
 else:cluster='';results['cluster']={'state':'not_applicable'}
 cluster_id='';quorate=False
 for line in cluster.splitlines():
  if line.startswith('Name:'):cluster_id=line.split(':',1)[1].strip()
  if line.startswith('Quorate:'):quorate=line.split(':',1)[1].strip().lower()=='yes'
 details={'addresses':run('addresses',['ip','-j','addr','show'],parse=True),'routes':run('routes',['ip','-j','route','show'],parse=True),
  'packages':run('packages',['dpkg-query','-W','-f=${binary:Package} ${Version}\n']), 'manual_packages':run('manual_packages',['apt-mark','showmanual']),
  'storage':run('storage',['pvesm','status']), 'zfs':run('zfs',['zpool','list','-Hp'],optional=True), 'lvm':run('lvm',['vgs','--reportformat','json'],optional=True),
  'guests':run('guests',['qm','list']), 'containers':run('containers',['pct','list']), 'cluster':cluster,
  'pci':run('pci',['lspci','-nn']), 'boot':run('boot',['proxmox-boot-tool','status']),
  'capabilities':{'restore_protocol':2,'journal':True}}
 if (root/'etc/pve/ceph.conf').exists():details['ceph']=run('ceph',['ceph','status','--format','json'])
 else:details['ceph']='';results['ceph']={'state':'not_applicable'}
 hashes={};metas={};walk_errors=[]
 def walk_error(exc):walk_errors.append(str(exc))
 for base in ('etc','usr/local'):
  start=root/base
  if not start.exists():continue
  for folder,dirs,files in os.walk(start,followlinks=False,onerror=walk_error):
   links=[x for x in dirs if (pathlib.Path(folder)/x).is_symlink()];dirs[:]=[x for x in dirs if x not in links]
   for name in files+links:
    p=pathlib.Path(folder)/name;rel=p.relative_to(root).as_posix()
    try:
     e=entry_meta(p,rel)
     if e is None:hashes[rel]='unsupported';continue
     metas[rel]={k:e[k] for k in ('type','mode','uid','gid','xattrs')}
     if 'link' in e:metas[rel]['link']=e['link']
     if e.get('metadata_warning'):metas[rel]['metadata_warning']=e['metadata_warning'];walk_errors.append(rel+': '+e['metadata_warning'])
     if e['type']=='symlink':hashes[rel]='symlink:'+e['link']
     elif e['size']<=MAX_FILE:hashes[rel]=digest(p.read_bytes())
     else:hashes[rel]='too-large'
    except OSError as exc:hashes[rel]='unreadable';walk_errors.append(rel+': '+str(exc))
 details['file_hashes']=hashes;details['file_metadata']=metas
 results['file_inventory']={'state':'failed','error':'; '.join(walk_errors)[:4096]} if walk_errors else {'state':'ok'}
 for key,getter,attribute in (('users',pwd.getpwall,'pw_uid'),('groups',grp.getgrall,'gr_gid')):
  try:
   details[key]={str(getattr(x,attribute)):getattr(x,'pw_name' if key=='users' else 'gr_name') for x in getter()};results[key]={'state':'ok'}
  except OSError as exc:details[key]={};results[key]={'state':'failed','error':str(exc)}
 try:details['boot_id']=(root/'proc/sys/kernel/random/boot_id').read_text().strip();results['boot_id']={'state':'ok'}
 except OSError as exc:details['boot_id']='';results['boot_id']={'state':'failed','error':str(exc)}
 try:debian=(root/'etc/debian_version').read_text().strip()
 except OSError:debian=''
 details['command_results']=results
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
 except (OSError,AttributeError) as exc:
  if not isinstance(exc,OSError) or exc.errno not in (errno.ENOTSUP,errno.EOPNOTSUPP):e['metadata_warning']=str(exc)
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
   if e.get('metadata_warning'):warnings.append('Metadata capture incomplete: /'+rel+': '+e.pop('metadata_warning'))
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
 warnings.extend(dependency_warnings(out,entries,root))
 return {'inventory':probe(root,test_mode),'entries':entries,'warnings':warnings}

def dependency_warnings(out,entries,root=None):
 """Bounded discovery of Proxmox hooks and explicit config/secret file references."""
 known={e['path'] for e in entries};warnings=[];storage={}
 p=out/'files/etc/pve/storage.cfg'
 if p.exists():
  current=None
  for line in p.read_text(errors='replace').splitlines():
   match=re.match(r'^dir:\s*(\S+)',line)
   if match:current=match.group(1)
   elif line and not line[0].isspace():current=None
   elif current:
    match=re.match(r'^\s+path\s+(\S+)',line)
    if match:storage[current]=match.group(1)
 for e in entries:
  if e.get('type')!='file' or not e['path'].startswith('etc/'):continue
  if e.get('size',0)>1024*1024:continue
  p=out/'files'/e['path']
  try:data=p.read_text(errors='replace')
  except OSError:continue
  if '\x00' in data:continue
  vm_config=re.fullmatch(r'etc/pve/(?:nodes/[^/]+/)?qemu-server/\d+\.conf',e['path']) is not None
  vim_config=e['path'].startswith('etc/vim/');vim_guards=[]
  for line in data.splitlines():
   line=line.strip()
   if not line or line.startswith(('#',';')) or (vim_config and line.startswith('"')):continue
   if vim_config:
    if re.match(r'^if\s+',line):
     guard=re.fullmatch(r'if\s+filereadable\(\s*(["\'])(/[^"\']+)\1\s*\)',line)
     vim_guards.append(guard.group(2) if guard else None)
    elif re.match(r'^(?:else|elseif)\b',line) and vim_guards:vim_guards[-1]=None
    elif re.match(r'^endif\b',line) and vim_guards:vim_guards.pop()
   hook=re.match(r'^hookscript\s*:\s*(\S+)',line)
   if hook:
    ref=hook.group(1);parts=ref.split(':',1)
    if len(parts)==2 and parts[0] in storage:
     candidate=storage[parts[0]].rstrip('/')+'/'+parts[1]
     if candidate.lstrip('/') not in known:warnings.append('Referenced hookscript is not captured: '+candidate+' ('+e['path']+')')
    else:warnings.append('Unresolved hookscript dependency: '+ref+' ('+e['path']+')')
   match=re.search(r'(?i)\b(keyfile|key_file|ssl_keyfile|privatekeyfile|certificatefile|certfile|ca_file|credentials|secret_file|include|source|script)\s*(?:[:=]\s*|\s+)["\']?(/[^\s"\';]+)',line)
   if match:
    directive=match.group(1).lower();candidate=match.group(2)
    if '*' in candidate or '?' in candidate:continue
    # RNG source selects a runtime device, not a restorable config file.
    if vm_config and directive=='source' and re.fullmatch(r'rng0\s*:\s*source=/dev/(?:urandom|random|hwrng)(?:,(?:max_bytes|period)=\d+)*',line):continue
    # Only absence proven on the source host makes a guarded Vim source optional.
    # lstat preserves warnings for broken links and never reads device contents.
    if vim_config and directive=='source' and re.match(r'^source\s+',line) and '|' not in line and candidate in vim_guards and root is not None:
     try:(root/candidate.lstrip('/')).lstat()
     except FileNotFoundError:continue
     except OSError:pass
    if candidate.lstrip('/') not in known:warnings.append('Referenced config/secret is not captured: '+candidate+' ('+e['path']+')')
 return sorted(set(warnings))

class RestoreError(OSError):
 """Failed mutation with its durable recovery result and original cause."""
 def __init__(self,cause,result):
  super().__init__(str(cause));self.result=result


def metadata(p,test_mode=False):
 e=entry_meta(p,'')
 if not e or e['type']!='file':raise ValueError('restore target must be a regular file')
 if e.get('metadata_warning') and not (test_mode and not hasattr(os,'listxattr')):raise ValueError('target metadata unavailable: '+e['metadata_warning'])
 return {k:e[k] for k in ('type','mode','uid','gid','xattrs')}


def file_state(root,rel,test_mode=False):
 p=confined(root,rel)
 try:meta=metadata(p,test_mode)
 except FileNotFoundError:return 'missing',None
 if p.stat().st_size>MAX_FILE:raise ValueError('target file too large')
 return digest(p.read_bytes()),meta


def protected_restore(path):
 prefixes=('usr/local/lib/anker/','etc/sudoers.d/','etc/pam.d/','etc/pve/','etc/corosync/','etc/ssh/','etc/apt/','etc/default/grub','etc/kernel/','etc/modprobe.d/','etc/udev/','etc/systemd/system/','etc/network/')
 names=('etc/passwd','etc/shadow','etc/group','etc/gshadow','etc/fstab','etc/machine-id','etc/hostname','etc/hosts','etc/anker-host.json','etc/sudoers','etc/nsswitch.conf')
 return path in names or path.startswith(prefixes) or not path.startswith(('etc/','usr/local/'))


def operation_id(value):
 if not isinstance(value,str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,127}',value):raise ValueError('invalid operation ID')
 return value


def operation_path(root,plan_id):
 return confined(pathlib.Path(root),'var/lib/anker-host/rollback/'+operation_id(plan_id))


@contextlib.contextmanager
def mutation_lock(root):
 folder=confined(root,'var/lib/anker-host');folder.mkdir(parents=True,exist_ok=True,mode=0o700)
 sync_ancestors(root,folder)
 fd=os.open(folder/'restore.lock',os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW,0o600)
 try:
  fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
  yield
 finally:os.close(fd)


def durable_json(path,value):
 # The initial exclusive file also leaves an inspectable marker if killed early.
 content=json.dumps(value,indent=2).encode()
 if not path.exists():
  fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
  with os.fdopen(fd,'wb') as f:f.write(content);f.flush();os.fsync(f.fileno())
 else:
  fd,tmp=tempfile.mkstemp(prefix='.journal-',dir=path.parent)
  try:
   with os.fdopen(fd,'wb') as f:f.write(content);f.flush();os.fsync(f.fileno())
   os.replace(tmp,path)
  finally:
   try:
    if os.path.exists(tmp):os.unlink(tmp)
   except OSError:pass
 sync_directory(path.parent)


def save_journal(folder,journal):durable_json(folder/'journal.json',journal)


def result_of(folder,journal):
 r={'state':journal['state'],'operation_id':journal['operation_id'],'applied':[e['path'] for e in journal['files'] if e.get('applied')], 'rollback_path':str(folder),'checks':journal.get('checks',[])+[e['path']+': '+e['phase']+(' (replacement outcome requires inspection)' if e['phase']=='writing' else '') for e in journal['files']],'reboot_verified':False}
 if journal.get('error'):r['error']=journal['error']
 return r


def load_journal(folder):
 if not folder.exists():raise ValueError('operation journal not found')
 try:
  if not (folder/'journal.json').exists():raise ValueError('operation journal missing: legacy helper rollback directory or interrupted journal creation; manual inspection required')
  j=json.loads((folder/'journal.json').read_text())
  if j['operation_id']!=folder.name or j.get('version')!=2:raise ValueError('invalid operation journal')
  if j['state'] not in ('writing','applied','rolling_back','rolled_back','interrupted','rollback_conflict','failed') or not isinstance(j['files'],list):raise ValueError('invalid journal state')
  for e in j['files']:
   relative(e['path'])
   if protected_restore(e['path']) or e['phase'] not in ('prepared','writing','applied','rolling_back','rolled_back','rollback_conflict'):raise ValueError('invalid journal file')
   for key in ('before_sha','after_sha'):
    if not re.fullmatch(r'[0-9a-f]{64}',e[key]) and not (key=='before_sha' and e[key]=='missing'):raise ValueError('invalid journal hash')
   if e['before_sha']=='missing' and e['before_metadata'] is not None:raise ValueError('invalid journal metadata')
  allowed={'applied':('applied',),'failed':('prepared',),'rolled_back':('prepared','rolled_back')}
  if j['state'] in allowed and any(e['phase'] not in allowed[j['state']] for e in j['files']):raise ValueError('inconsistent journal terminal state')
  return j
 except (OSError,ValueError,KeyError,TypeError) as exc:raise RestoreError(exc,{'state':'interrupted','operation_id':folder.name,'applied':[],'rollback_path':str(folder),'checks':['journal unreadable; operator inspection required'],'reboot_verified':False}) from exc


def inspect_journal(folder):
 j=load_journal(folder)
 if j['state'] in ('writing','rolling_back'):
  j['state']='interrupted';j['checks'].append('execution interrupted; inspect write-ahead file phases');save_journal(folder,j)
 return j


def restore_status(root,plan_id):
 root=pathlib.Path(root);folder=operation_path(root,plan_id)
 with mutation_lock(root):
  if not folder.exists():return {'state':'not_found','operation_id':plan_id,'applied':[],'rollback_path':str(folder),'checks':['no operation directory exists; no host write was started for this ID'],'reboot_verified':False}
  return result_of(folder,inspect_journal(folder))


def write_file(path,data,mode,uid,gid,test_mode):
 fd,tmp=tempfile.mkstemp(prefix='.anker-',dir=path.parent)
 try:
  with os.fdopen(fd,'wb') as f:
   f.write(data);f.flush()
   if not test_mode:os.fchown(f.fileno(),uid,gid)
   os.fchmod(f.fileno(),mode);os.fsync(f.fileno())
  if metadata(pathlib.Path(tmp),test_mode)['xattrs']:raise ValueError('staging metadata contains unsupported ACL/xattrs; manual procedure required')
  sync_directory(path.parent)
  return tmp
 except BaseException:
  try:
   if os.path.exists(tmp):os.unlink(tmp)
  except OSError:pass
  raise


def rollback_journal(root,folder,j,test_mode=False,retry_conflicts=False):
 # Explicit retry remains conditional: only our after state or original before
 # state is accepted; an unresolved foreign state is preserved again.
 j['state']='rolling_back';save_journal(folder,j);conflict=False
 for e in reversed(j['files']):
  if e['phase'] not in ('writing','applied','rolling_back') and not (retry_conflicts and e['phase']=='rollback_conflict'):continue
  try:
   p=confined(root,e['path']);current=file_state(root,e['path'],test_mode)
   if current==(e['before_sha'],e['before_metadata']):e['phase']='rolled_back';save_journal(folder,j);continue
   if current!=(e['after_sha'],e['after_metadata']):
    e['phase']='rollback_conflict';conflict=True;save_journal(folder,j);continue
   e['phase']='rolling_back';save_journal(folder,j)
   if e['before_sha']=='missing':
    # Recheck immediately before unlink, just as for replacement.
    if file_state(root,e['path'],test_mode)!=current:raise ValueError('rollback target changed: '+e['path'])
    p.unlink();sync_directory(p.parent)
   else:
    backup=confined(folder,'before/'+e['path']);data=backup.read_bytes()
    if digest(data)!=e['before_sha']:raise ValueError('rollback backup integrity failure')
    m=e['before_metadata'];tmp=write_file(p,data,m['mode'],m['uid'],m['gid'],test_mode)
    try:
     if file_state(root,e['path'],test_mode)!=current:raise ValueError('rollback target changed: '+e['path'])
     os.replace(tmp,p);sync_directory(p.parent)
    finally:
     try:
      if os.path.exists(tmp):os.unlink(tmp)
     except OSError:pass
   if file_state(root,e['path'],test_mode)!=(e['before_sha'],e['before_metadata']):raise ValueError('rollback verification failed: '+e['path'])
   e['phase']='rolled_back';save_journal(folder,j)
  except Exception as rollback_error:
   e['phase']='rollback_conflict';conflict=True;j['checks'].append(e['path']+': rollback failed: '+str(rollback_error));save_journal(folder,j)
 if any(e['phase']=='rollback_conflict' for e in j['files']):conflict=True
 j['state']='rollback_conflict' if conflict else 'rolled_back';save_journal(folder,j)
 return result_of(folder,j)


def rollback_files(root,plan_id,confirm,test_mode=False):
 root=pathlib.Path(root);folder=operation_path(root,plan_id)
 if confirm!=plan_id:raise ValueError('explicit plan confirmation required')
 with mutation_lock(root):
  j=inspect_journal(folder)
  prior_error=j.pop('error',None)
  if prior_error:
   j.setdefault('initial_error',prior_error)
   j['checks'].append('earlier operation error: '+prior_error)
  try:
   result=rollback_journal(root,folder,j,test_mode,retry_conflicts=True)
   if j['state']=='rollback_conflict':
    j['error']='rollback conflict: target content or metadata differs from this operation';save_journal(folder,j);result=result_of(folder,j)
   return result
  except Exception as exc:
   j['state']='rollback_conflict';j['error']=str(exc)
   try:save_journal(folder,j)
   except Exception as journal_error:j['checks'].append('journal update failed: '+str(journal_error))
   raise RestoreError(exc,result_of(folder,j)) from exc


def apply_files(root,items,test_mode=False,plan_id=None):
 """Per-file verified replacement; flock coordinates helpers, not arbitrary writers.
 An external writer can still race the final check/rename. No global host atomicity.
 """
 root=pathlib.Path(root);plan_id=operation_id(('local-'+str(time.time_ns())) if plan_id is None else plan_id)
 folder=operation_path(root,plan_id)
 with mutation_lock(root):
  if folder.exists():
   j=inspect_journal(folder);raise RestoreError('operation already exists; inspect or rollback, never replay',result_of(folder,j))
  if folder.parent.exists():
   for older in sorted(folder.parent.iterdir()):
    if not older.is_dir():continue
    operation_path(root,older.name)
    old_j=inspect_journal(older)
    if old_j['state'] in ('writing','rolling_back','interrupted','rollback_conflict'):
     raise RestoreError('unresolved earlier operation '+older.name+'; inspect and rollback first',result_of(older,old_j))
  prepared=[];seen=set()
  for item in items:
   rel=item['path'];p=confined(root,rel)
   if protected_restore(rel):raise ValueError('protected restore requires manual procedure: '+rel)
   if rel in seen:raise ValueError('duplicate restore path')
   seen.add(rel)
   if item.get('type','file')!='file' or item.get('xattrs'):raise ValueError('file metadata requires manual procedure')
   before,meta=file_state(root,rel,test_mode)
   if item.get('before_sha')!=before:raise ValueError('target file changed: '+rel)
   if not test_mode and 'expected_metadata' not in item:raise ValueError('expected target metadata required')
   expected=item.get('expected_metadata',meta)
   if expected!=meta:raise ValueError('target metadata changed: '+rel)
   if meta and meta['xattrs']:raise ValueError('target ACL/xattrs require manual procedure')
   if not test_mode:
    for key in ('type','mode','uid','gid','user','group'):
     if key not in item:raise ValueError('restore field required: '+key)
    try:
     if pwd.getpwuid(item['uid']).pw_name!=item['user'] or grp.getgrgid(item['gid']).gr_name!=item['group']:raise ValueError('target owner identity changed')
    except KeyError as exc:raise ValueError('target owner identity unknown') from exc
   mode=item.get('mode',0o600);uid=item.get('uid',os.getuid());gid=item.get('gid',os.getgid())
   if any(not isinstance(x,int) or isinstance(x,bool) for x in (mode,uid,gid)) or mode<0 or mode>0o777 or uid<0 or gid<0:raise ValueError('invalid restore permissions')
   data=base64.b64decode(item['content'],validate=True)
   if len(data)>MAX_FILE:raise ValueError('restore file too large')
   prepared.append((p,item,data,meta,before))
  if not prepared:raise ValueError('no restore files')
  folder.mkdir(parents=True,mode=0o700);sync_ancestors(root,folder)
  j={'version':2,'operation_id':plan_id,'state':'writing','files':[],'checks':['helper lock held; external writers require an operator change freeze','service and reboot checks require operator']}
  stages=[]
  try:
   save_journal(folder,j)
   for p,item,data,meta,before in prepared:
    rel=item['path'];p.parent.mkdir(parents=True,exist_ok=True);confined(root,rel);sync_ancestors(root,p.parent)
    old=p.read_bytes() if before!='missing' else None
    # Validate the bytes actually backed up, not a stale initial hash.
    if file_state(root,rel,test_mode)!=(before,meta) or (old is not None and digest(old)!=before):raise ValueError('target changed during rollback capture: '+rel)
    if old is not None:
     backup=confined(folder,'before/'+rel);backup.parent.mkdir(parents=True,exist_ok=True)
     with open(backup,'xb') as f:f.write(old);f.flush();os.fsync(f.fileno())
     sync_directory(backup.parent)
    stage=write_file(p,data,item.get('mode',0o600),item.get('uid',0),item.get('gid',0),test_mode);stages.append(stage)
    after_meta=metadata(pathlib.Path(stage),test_mode)
    j['files'].append({'path':rel,'before_sha':before,'before_metadata':meta,'after_sha':digest(data),'after_metadata':after_meta,'stage':str(stage),'phase':'prepared','applied':False})
   # Every rollback file, staging file and metadata record is durable before writes.
   for directory,_,_ in os.walk(folder,topdown=False):sync_directory(pathlib.Path(directory))
   save_journal(folder,j)
   for e in j['files']:
    p=confined(root,e['path'])
    e['phase']='writing';save_journal(folder,j)
    if file_state(root,e['path'],test_mode)!=(e['before_sha'],e['before_metadata']):raise ValueError('target changed before replace: '+e['path'])
    if digest(pathlib.Path(e['stage']).read_bytes())!=e['after_sha'] or metadata(pathlib.Path(e['stage']),test_mode)!=e['after_metadata']:raise ValueError('staged file integrity failure')
    # Revalidate after reading the staged bytes and immediately before rename.
    if file_state(root,e['path'],test_mode)!=(e['before_sha'],e['before_metadata']):raise ValueError('target changed before replace: '+e['path'])
    os.replace(e['stage'],p);e['applied']=True;sync_directory(p.parent)
    if file_state(root,e['path'],test_mode)!=(e['after_sha'],e['after_metadata']):raise ValueError('replacement verification failed: '+e['path'])
    e['phase']='applied';save_journal(folder,j)
   j['state']='applied';j['checks'].append('all replacement contents and metadata verified');save_journal(folder,j)
   return result_of(folder,j)
  except Exception as exc:
   j['error']=str(exc)
   try:
    if any(e['phase'] in ('writing','applied') for e in j['files']):rollback_journal(root,folder,j,test_mode)
    else:j['state']='failed';save_journal(folder,j)
   except Exception as rollback_error:
    j['state']='rollback_conflict';j['checks'].append('rollback or journal failure: '+str(rollback_error))
    try:save_journal(folder,j)
    except Exception as journal_error:j['checks'].append('journal update failed: '+str(journal_error))
   raise RestoreError(exc,result_of(folder,j)) from exc
  finally:
   for stage in stages:
    try:
     if os.path.exists(stage):os.unlink(stage)
    except OSError as cleanup_error:j['checks'].append('staging cleanup failed: '+str(cleanup_error))

def sync_ancestors(root,path):
 root=pathlib.Path(root);path=pathlib.Path(path)
 while True:
  sync_directory(path)
  if path==root:break
  if root not in path.parents:raise ValueError('directory outside restore root')
  path=path.parent

def sync_directory(path):
 fd=os.open(path,os.O_RDONLY)
 try:os.fsync(fd)
 finally:os.close(fd)

def authorize(operation,read_only=False):
 if operation not in ("probe","collect","apply","restore-status","rollback"):raise ValueError("unknown operation")
 if read_only and operation in ("apply","restore-status","rollback"):raise ValueError("read-only backup authority cannot restore")

def main(root=None):
 os.umask(0o077)
 if os.geteuid()!=0 or sys.platform!='linux':raise ValueError('host helper requires root on Linux')
 if sys.argv[1:] not in ([],['--read-only']):raise ValueError('invalid arguments')
 read_only=sys.argv[1:]==['--read-only']
 data=sys.stdin.buffer.readline(MAX_REQUEST+1)
 if len(data)>MAX_REQUEST:raise ValueError('request too large')
 request=json.loads(data);op=request.get('operation');authorize(op,read_only)
 if request.get('version')!=1:raise ValueError('unsupported protocol version')
 root=pathlib.Path('/') if root is None else pathlib.Path(root)
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
 elif op=='restore-status':print(json.dumps(restore_status(root,request['plan_id'])))
 elif op=='rollback':print(json.dumps(rollback_files(root,request['plan_id'],request.get('confirm'))))
 elif op=='apply':
  if not request.get('confirm') or request.get('confirm')!=request.get('plan_id'):raise ValueError('explicit plan confirmation required')
  operation_id(request.get('plan_id'))
  if any('expected_metadata' not in e for e in request.get('files',[])):raise ValueError('expected target metadata required')
  expected=dict(request['expected_inventory']);current=probe(root)
  required=('pve_version','users','groups','file_inventory')
  for inv in (expected,current):
   details=inv.get('details',{})
   if details.get('capabilities',{}).get('restore_protocol')!=2 or details.get('capabilities',{}).get('journal') is not True:raise ValueError('hardened restore capabilities required')
   if any(details.get('command_results',{}).get(k,{}).get('state')!='ok' for k in required):raise ValueError('required target inventory query failed')
  for x in ('captured_at','fingerprint'):expected.pop(x,None);current.pop(x,None)
  stable=('file_hashes','file_metadata','users','groups','capabilities','command_results','boot_id','addresses','routes','packages','manual_packages','pci','boot')
  for inv in (expected,current):inv['details']={k:v for k,v in inv.get('details',{}).items() if k in stable}
  def normalize(v):
   if isinstance(v,dict):return {k:normalize(x) for k,x in v.items() if k not in ('valid_life_time','preferred_life_time') and x not in ('',None,[],{})}
   if isinstance(v,list):return [normalize(x) for x in v]
   return v
  if normalize(expected)!=normalize(current):raise ValueError('target inventory changed')
  print(json.dumps(apply_files(root,request['files'],plan_id=request['plan_id'])))
 else:raise ValueError('unknown operation')
if __name__=='__main__':
 try:main()
 except Exception as exc:
  result=getattr(exc,'result',{'state':'failed','operation_id':'','applied':[],'rollback_path':'','checks':[],'reboot_verified':False,'error':str(exc)})
  print(json.dumps(result))
  print('Anker host operation failed: '+str(exc),file=sys.stderr);sys.exit(1)
