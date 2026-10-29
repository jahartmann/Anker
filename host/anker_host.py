#!/usr/bin/env python3
"""Anker host envelope v1, hardened restore protocol v2. Install root-owned; stdin JSON, stdout JSON or tar.
No daemon, no user-supplied commands, no shell interpolation.
"""
import ipaddress, contextlib, fcntl, pwd, grp, re, errno, base64, hashlib, json, os, pathlib, shutil, socket, selectors, sqlite3, stat, subprocess, sys, tarfile, tempfile, time

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

def host_type(root):
 # PBS takes precedence on combined installations; client-only packages do not qualify.
 return 'pbs' if (pathlib.Path(root)/'etc/proxmox-backup').is_dir() else 'pve'

def pbs_sections(root,filename,kind,fields):
 p=confined(pathlib.Path(root),'etc/proxmox-backup/'+filename)
 if not p.exists():return []
 if p.stat().st_size>1024*1024:raise ValueError('PBS configuration exceeds inventory limit')
 items=[];current=None
 for line in p.read_text().splitlines():
  if not line.strip() or line.lstrip().startswith('#'):continue
  if not line[0].isspace():
   match=re.fullmatch(re.escape(kind)+r':\s*(\S+)\s*',line)
   if not match:raise ValueError('invalid PBS configuration section')
   current={'name':match.group(1)};items.append(current)
  elif current is not None:
   pair=line.strip().split(None,1)
   if len(pair)==2 and pair[0] in fields:
    if pair[0] in current:raise ValueError('duplicate PBS option')
    current[pair[0]]=pair[1]
  else:raise ValueError('PBS option without section')
 return items

def pbs_data_roots(root):
 roots=[]
 for item in pbs_sections(root,'datastore.cfg','datastore',{'path','backing-device'}):
  value=item.get('path','')
  if not value:raise ValueError('PBS datastore path missing')
  if not value.startswith('/'):
   # Removable datastores live below /mnt/datastore, outside the PBS config roots.
   if not item.get('backing-device'):raise ValueError('relative PBS datastore path without backing device')
   continue
  candidate=pathlib.Path(root)/value.lstrip('/')
  roots.extend([candidate,candidate.resolve()])
 return roots

def pbs_excluded(p,data_roots):
 if any((parent/'.chunks').is_dir() for parent in ([p] if p.is_dir() else [])+list(p.parents)):return True
 return any(p==base or base in p.parents or p.resolve()==base or base in p.resolve().parents for base in data_roots)

def pbs_capture_paths(paths):
 # PBS support is deliberately configuration-only, even with an older/custom host profile.
 for path in paths:
  relative(path.lstrip('/'))
  if not any(path==base or path.startswith(base+'/') for base in ('/etc','/usr/local')):
   raise ValueError('PBS configuration capture only permits /etc and /usr/local')
 return list(dict.fromkeys(list(paths)+['/etc/proxmox-backup']))

PBS_RESTORE_FILES=frozenset(('acl.cfg','user.cfg','remote.cfg','node.cfg','datastore.cfg','sync.cfg','prune.cfg','verification.cfg','domains.cfg','notifications.cfg','notifications-priv.cfg','s3.cfg','traffic-control.cfg','media-pool.cfg','tape.cfg','authkey.key','authkey.pub','csrf.key','proxy.key','proxy.pem','shadow.json','token.shadow','tfa.json'))
PBS_SERVICES=('proxmox-backup','proxmox-backup-proxy')

def pbs_restore_path(path):
 return path.startswith('etc/proxmox-backup/') and path[len('etc/proxmox-backup/'):] in PBS_RESTORE_FILES

def pbs_service_states():
 states={}
 for name in PBS_SERVICES:
  result=subprocess.run(['systemctl','show','--property=ActiveState','--value',name],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=10,check=False)
  value=result.stdout.decode().strip()
  if result.returncode!=0 or value not in ('active','inactive','activating','deactivating','failed','reloading'):raise ValueError('PBS service state unavailable')
  states[name]=value
 return states

def pbs_datastore_identity(root):
 # Stat directory identities only. Never list, open, create or modify backup contents.
 result={};root=pathlib.Path(root)
 mountinfo=(root/'proc/self/mountinfo').read_text()
 config_path=pathlib.PurePosixPath('/etc/proxmox-backup');journal_path=pathlib.PurePosixPath('/var/lib/anker-host')
 mounts=[]
 for line in mountinfo.splitlines():
  fields=line.split()
  if len(fields)<6:raise ValueError('invalid mount evidence')
  mount=fields[4]
  for encoded,decoded in (('\\040',' '),('\\011','\t'),('\\134','\\')):mount=mount.replace(encoded,decoded)
  mount_path=pathlib.PurePosixPath(mount)
  if str(mount_path)!='/' and fields[3]!='/' and (mount_path==config_path or mount_path in config_path.parents or config_path in mount_path.parents):raise ValueError('PBS configuration mount requires manual recovery; possible datastore alias')
  if str(mount_path)!='/' and fields[3]!='/' and (mount_path==journal_path or mount_path in journal_path.parents or journal_path in mount_path.parents):raise ValueError('PBS recovery journal mount requires manual recovery; possible datastore alias')
  mounts.append((fields[0],mount_path))
 for item in pbs_sections(root,'datastore.cfg','datastore',{'path','backing-device','backend'}):
  path=item.get('path','')
  if not path.startswith('/'):raise ValueError('mount removable PBS datastore before recovery')
  p=confined(root,path.lstrip('/'));chunks=confined(root,(path.rstrip('/')+'/.chunks').lstrip('/'))
  st=p.stat();chunk_st=chunks.stat()
  if not stat.S_ISDIR(st.st_mode) or not stat.S_ISDIR(chunk_st.st_mode):raise ValueError('existing PBS datastore directories required')
  candidates=[(ident,mount) for ident,mount in mounts if mount==pathlib.PurePosixPath(path) or mount in pathlib.PurePosixPath(path).parents]
  if not candidates:raise ValueError('PBS datastore mount evidence missing')
  mount_id,mount=max(candidates,key=lambda x:len(x[1].parts))
  result[item['name']]={'path':path,'device':st.st_dev,'inode':st.st_ino,'chunks_device':chunk_st.st_dev,'chunks_inode':chunk_st.st_ino,'mount_id':mount_id,'mount':str(mount)}
 return result

def pbs_quiescent_identity(root):
 if host_type(root)!='pbs':raise ValueError('PBS recovery requires a PBS target')
 if pbs_excluded(pathlib.Path(root)/'etc/proxmox-backup',pbs_data_roots(root)):raise ValueError('PBS configuration directory overlaps datastore')
 if pbs_excluded(pathlib.Path(root)/'var/lib/anker-host',pbs_data_roots(root)):raise ValueError('PBS recovery journal must be outside all datastores')
 if any(state!='inactive' for state in pbs_service_states().values()):raise ValueError('stop proxmox-backup and proxmox-backup-proxy before PBS recovery or rollback')
 return pbs_datastore_identity(root)

def pbs_parse_restore_config(data,kind):
 sections={};current=None
 for line in data.decode('utf-8').splitlines():
  if not line.strip() or line.lstrip().startswith('#'):continue
  if not line[0].isspace():
   match=re.fullmatch(re.escape(kind)+r':\s*([A-Za-z0-9_][A-Za-z0-9_.-]*)\s*',line)
   if not match or match.group(1) in sections:raise ValueError('invalid PBS restore configuration section')
   current={};sections[match.group(1)]=current
  else:
   pair=line.strip().split(None,1)
   if current is None or len(pair)!=2:raise ValueError('invalid PBS restore configuration option')
   if pair[0] in current:
    if kind!='sync' or pair[0] not in ('group-filter','associated-key'):raise ValueError('invalid PBS restore configuration option')
    current[pair[0]]+='\n'+pair[1]
   else:current[pair[0]]=pair[1]
 return sections

def validate_pbs_restore_content(root,path,data):
 name=path.rsplit('/',1)[-1]
 kinds={'datastore.cfg':'datastore','sync.cfg':'sync','prune.cfg':'prune','verification.cfg':'verification'}
 if name not in kinds:return
 sections=pbs_parse_restore_config(data,kinds[name])
 target_file=confined(pathlib.Path(root),'etc/proxmox-backup/datastore.cfg')
 target=pbs_parse_restore_config(target_file.read_bytes() if target_file.exists() else b'','datastore')
 if name=='datastore.cfg':
  if set(sections)!=set(target):raise ValueError('PBS datastore membership must be preserved; attach existing datastores separately')
  for ident,options in sections.items():
   for field in ('path','backing-device','backend','maintenance-mode'):
    if options.get(field)!=target[ident].get(field):raise ValueError('PBS datastore backing or maintenance mode must be preserved')
   if any(key in options for key in ('gc-schedule','prune-schedule','verify-new','gc-on-unmount')):raise ValueError('PBS datastore automatic actions must remain disabled')
 else:
  for options in sections.values():
   if name=='sync.cfg' and not options.get('remote-store'):raise ValueError('PBS sync job requires remote-store')
   if name=='prune.cfg':
    if not options.get('schedule'):raise ValueError('PBS prune job requires a saved schedule')
    if options.get('disable') not in ('true','1'):raise ValueError('PBS prune jobs must remain disabled')
   elif 'schedule' in options or 'run-on-mount' in options or 'unmount-on-done' in options:raise ValueError('PBS job triggers must remain disabled')
   if options.get('store') not in target:raise ValueError('PBS job references an unavailable target datastore')

def pbs_journal_guard(root,j):
 if j.get('platform')=='pbs' or any(e['path'].startswith('etc/proxmox-backup/') for e in j['files']):
  if any(not pbs_restore_path(e['path']) for e in j['files']):raise ValueError('PBS journal contains non-configuration paths')
  if any(pbs_excluded(pathlib.Path(root)/e['path'],pbs_data_roots(root)) for e in j['files']):raise ValueError('PBS journal path overlaps datastore')
  if j.get('pbs_datastore_identity')!=pbs_quiescent_identity(root):raise ValueError('PBS datastore identity changed since recovery')

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
 platform=host_type(root);data_roots=pbs_data_roots(root) if platform=='pbs' else []
 if platform=='pbs':
  version=run('pbs_version',['dpkg-query','-W','-f=${Version}','proxmox-backup-server']).strip()
 else:
  version=run('pve_version',['pveversion']).strip();version=version.split('/')[1] if '/' in version else version
 def pve_run(key,args,**kwargs):
  if platform=='pbs':results[key]={'state':'not_applicable'};return ''
  return run(key,args,**kwargs)
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
 if platform=='pve' and (root/'etc/pve/corosync.conf').exists():cluster=run('cluster',['pvecm','status'])
 else:cluster='';results['cluster']={'state':'not_applicable'}
 cluster_id='';quorate=False
 for line in cluster.splitlines():
  if line.startswith('Name:'):cluster_id=line.split(':',1)[1].strip()
  if line.startswith('Quorate:'):quorate=line.split(':',1)[1].strip().lower()=='yes'
 details={'addresses':run('addresses',['ip','-j','addr','show'],parse=True),'routes':run('routes',['ip','-j','route','show'],parse=True),
  'packages':run('packages',['dpkg-query','-W','-f=${binary:Package} ${Version}\n']), 'manual_packages':run('manual_packages',['apt-mark','showmanual']),
  'storage':pve_run('storage',['pvesm','status']), 'zfs':run('zfs',['zpool','list','-Hp'],optional=True), 'lvm':run('lvm',['vgs','--reportformat','json'],optional=True),
  'guests':pve_run('guests',['qm','list']), 'containers':pve_run('containers',['pct','list']), 'cluster':cluster,
  'pci':run('pci',['lspci','-nn']), 'boot':pve_run('boot',['proxmox-boot-tool','status']),
  'capabilities':{'restore_protocol':2,'remove_files':True,'journal':True,'recovery_read_protocol':1,'recovery_verify_protocol':1}}
 if platform=='pve' and (root/'etc/pve/ceph.conf').exists():details['ceph']=run('ceph',['ceph','status','--format','json'])
 else:details['ceph']='';results['ceph']={'state':'not_applicable'}
 if platform=='pbs':
  details['capabilities']={'config_backup':True,'restore_protocol':2,'journal':True,'recovery_read_protocol':1,'recovery_verify_protocol':1,'pbs_restore_protocol':1}
  for key,query in (('pbs_services',pbs_service_states),('pbs_datastore_identity',lambda:pbs_datastore_identity(root))):
   try:details[key]=query();results[key]={'state':'ok'}
   except (OSError,ValueError,subprocess.TimeoutExpired):details[key]={};results[key]={'state':'failed'}
  for key,filename,kind,fields in (
   ('pbs_datastores','datastore.cfg','datastore',{'path','gc-schedule','prune-schedule'}),
   ('pbs_sync_jobs','sync.cfg','sync',{'store','schedule','sync-direction'}),
   ('pbs_prune_jobs','prune.cfg','prune',{'store','schedule','disable'}),
   ('pbs_verify_jobs','verification.cfg','verification',{'store','schedule'})):
   try:details[key]=pbs_sections(root,filename,kind,fields);results[key]={'state':'ok'}
   except (OSError,ValueError):details[key]=[];results[key]={'state':'failed'}
 hashes={};metas={};walk_errors=[]
 def walk_error(exc):walk_errors.append(str(exc))
 for base in ('etc','usr/local'):
  start=root/base
  if not start.exists() or (platform=='pbs' and (start.is_symlink() or pbs_excluded(start,data_roots))):continue
  for folder,dirs,files in os.walk(start,followlinks=False,onerror=walk_error):
   if platform=='pbs':
    dirs[:]=[x for x in dirs if not pbs_excluded(pathlib.Path(folder)/x,data_roots) and x!='.chunks']
    files[:]=[x for x in files if not pbs_excluded(pathlib.Path(folder)/x,data_roots)]
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
 identity=verification_identity(root);details['machine_identity_sha256']=identity['value'];results['machine_identity']={'state':identity['state']}
 try:details['boot_id']=(root/'proc/sys/kernel/random/boot_id').read_text().strip();results['boot_id']={'state':'ok'}
 except OSError as exc:details['boot_id']='';results['boot_id']={'state':'failed','error':str(exc)}
 try:debian=(root/'etc/debian_version').read_text().strip()
 except OSError:debian=''
 details['command_results']=results
 pbs_datastores=[{'name':x['name'],'path':x.get('path','')} for x in details.get('pbs_datastores',[])]
 pbs_jobs=[{'kind':kind,'name':x['name'],'store':x.get('store',''),'schedule':x.get('schedule',''),'disabled':x.get('disable') in ('true','1')} for kind,key in (('Sync','pbs_sync_jobs'),('Prune','pbs_prune_jobs'),('Verify','pbs_verify_jobs')) for x in details.get(key,[])]
 return {'hostname':socket.gethostname(),'pbs_datastores':pbs_datastores,'pbs_jobs':pbs_jobs,'host_type':platform,'pve_version':version if platform=='pve' else '', 'pbs_version':version if platform=='pbs' else '', 'debian':debian,'kernel':os.uname().release,'boot_mode':'UEFI' if (root/'sys/firmware/efi').exists() else 'BIOS','cluster_id':cluster_id,'quorate':quorate,'interfaces':net,'disks':disks,'details':details,'captured_at':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())}

def relative(path):
 if not isinstance(path,str) or not path:raise ValueError('invalid path: empty or non-text path')
 if path.startswith('/'):raise ValueError('invalid path: expected a relative path')
 if '\\' in path and not (os.sep=='/' and path.startswith(('etc/systemd/system/','etc/systemd/user/')) and '\\' not in re.sub(r'\\x[0-9a-fA-F]{2}','',path)):raise ValueError('invalid path: backslash is not supported')
 if '\x00' in path:raise ValueError('invalid path: NUL character is not supported')
 if any(x in ('','..','.') for x in path.split('/')):raise ValueError('invalid path: empty, dot or parent component is not allowed')
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

RECOVERY_MAX_PATHS = 4096
RECOVERY_MAX_FILE = 1024 * 1024
RECOVERY_MAX_TOTAL = 8 * 1024 * 1024
RECOVERY_MAX_METADATA = 8 * 1024 * 1024
RECOVERY_MAX_STORAGE_IDS = 32
RECOVERY_STORAGE_TIMEOUT = 20
RECOVERY_MAX_STORAGE_OUTPUT = 1024 * 1024

class RecoveryLimit(ValueError):
 pass

@contextlib.contextmanager
def recovery_open(root_fd,path):
 """Walk every component relative to pinned directories; never follow links."""
 relative(path);fds=[];parent=root_fd
 try:
  parts=path.split('/')
  for i,part in enumerate(parts):
   flags=os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK
   if i<len(parts)-1:flags|=os.O_DIRECTORY
   parent=os.open(part,flags,dir_fd=parent);fds.append(parent)
  yield parent
 finally:
  for fd in reversed(fds):os.close(fd)

def recovery_stat(st):
 return (st.st_dev,st.st_ino,st.st_mode,st.st_uid,st.st_gid,st.st_size,st.st_mtime_ns,st.st_ctime_ns,st.st_nlink)

def recovery_metadata(fd,path):
 st=os.fstat(fd);attrs={};size=0
 try:
  names=os.listxattr(fd)
  if len(names)>256:raise RecoveryLimit('extended metadata limit exceeded')
  for name in names:
   value=os.getxattr(fd,name);size+=len(name.encode())+len(value)
   if size>64*1024:raise RecoveryLimit('extended metadata limit exceeded')
   attrs[name]=base64.b64encode(value).decode()
 except OSError as exc:
  if exc.errno not in (errno.ENOTSUP,errno.EOPNOTSUPP):raise
 return {'path':path,'type':'file','mode':stat.S_IMODE(st.st_mode),'uid':st.st_uid,'gid':st.st_gid,'mtime':int(st.st_mtime),'size':st.st_size,'xattrs':attrs}

def recovery_content(fd,limit):
 chunks=[];size=0
 while True:
  data=os.read(fd,min(64*1024,limit+1-size))
  if not data:return b''.join(chunks)
  size+=len(data)
  if size>limit:return None
  chunks.append(data)

def validate_host_profile(paths):
 if not isinstance(paths,list):raise ValueError('invalid host profile paths')
 normalized=[]
 for path in paths:
  if not isinstance(path,str) or not path.startswith('/') or '..' in path or path in ('/','/proc','/sys','/dev','/var/lib'):raise ValueError('invalid host profile path')
  path=pathlib.PurePosixPath(path).as_posix()
  if path in ('/','/proc','/sys','/dev','/var/lib'):raise ValueError('invalid host profile path')
  relative(path[1:]);normalized.append(path)
 return normalized

def load_host_profile(root):
 """Both collection and recovery read use this root-owned authority file."""
 with contextlib.ExitStack() as stack:
  root_fd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);stack.callback(os.close,root_fd)
  try:fd=stack.enter_context(recovery_open(root_fd,'etc/anker-host.json'))
  except FileNotFoundError:return ['/etc','/usr/local']
  before=os.fstat(fd)
  if not stat.S_ISREG(before.st_mode) or before.st_uid!=0 or stat.S_IMODE(before.st_mode)&0o022:raise ValueError('host profile must be root-owned and not writable by others')
  raw=recovery_content(fd,64*1024)
  if raw is None:raise ValueError('host profile too large')
  if recovery_stat(before)!=recovery_stat(os.fstat(fd)):raise ValueError('host profile changed during read')
  cfg=json.loads(raw)
  if not isinstance(cfg,dict):raise ValueError('invalid host profile')
  return validate_host_profile(cfg.get('paths',['/etc','/usr/local']))

def read_recovery_file(root_fd,path,remaining):
 result={'path':path};opened=False
 try:
  with recovery_open(root_fd,path) as fd:
   opened=True
   before=os.fstat(fd)
   if not stat.S_ISREG(before.st_mode):return dict(result,state='unsupported',error='regular file required')
   if before.st_size>RECOVERY_MAX_FILE:return dict(result,state='too_large',error='file exceeds 1 MiB limit')
   if before.st_size>remaining:return dict(result,state='limit',error='total content exceeds 8 MiB limit')
   meta=recovery_metadata(fd,path);limit=min(RECOVERY_MAX_FILE,remaining);content=recovery_content(fd,limit)
   if content is None:return dict(result,state='too_large' if limit==RECOVERY_MAX_FILE else 'limit',error='content limit exceeded')
   os.lseek(fd,0,os.SEEK_SET);verify=recovery_content(fd,limit)
   after=os.fstat(fd);after_meta=recovery_metadata(fd,path)
   if recovery_stat(before)!=recovery_stat(after) or meta!=after_meta or content!=verify or len(content)!=before.st_size:
    return dict(result,state='changed',error='file changed during read')
   # Re-open the whole namespace to detect parent/target replacements while reading.
   try:
    with recovery_open(root_fd,path) as current:
     if recovery_stat(before)!=recovery_stat(os.fstat(current)):return dict(result,state='changed',error='file path changed during read')
   except OSError:return dict(result,state='changed',error='file path changed during read')
   sha=digest(content);meta['sha256']=sha
   return dict(result,state='read',sha256=sha,metadata=meta,content=base64.b64encode(content).decode())
 except FileNotFoundError:return dict(result,state='unreadable',error='file cannot be read') if opened else dict(result,state='missing')
 except OSError as exc:
  state='unsupported' if exc.errno in (errno.ELOOP,errno.ENOTDIR,errno.ENXIO,errno.ENODEV) else 'unreadable'
  return dict(result,state=state,error='path cannot be safely opened' if state=='unsupported' else 'file or metadata cannot be read')
 except RecoveryLimit:return dict(result,state='limit',error='extended metadata limit exceeded')
 except (ValueError,AttributeError):return dict(result,state='unreadable',error='file metadata cannot be read completely')

def recovery_storage_volumes(storage_id,deadline):
 """Only pvesm list with a validated ID; cap bytes while consuming its pipe."""
 if time.monotonic()>=deadline:return {'state':'failed','error':'storage listing timed out'}
 try:
  with subprocess.Popen(['pvesm','list',storage_id],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL) as process:
   with selectors.DefaultSelector() as selector:
    selector.register(process.stdout,selectors.EVENT_READ);chunks=[];size=0
    try:
     while True:
      left=deadline-time.monotonic()
      if left<=0 or not selector.select(left):raise subprocess.TimeoutExpired(process.args,RECOVERY_STORAGE_TIMEOUT)
      data=os.read(process.stdout.fileno(),64*1024)
      if not data:break
      size+=len(data)
      if size>RECOVERY_MAX_STORAGE_OUTPUT:
       process.kill();process.wait();return {'state':'limit','error':'storage listing exceeds output limit'}
      chunks.append(data)
     if process.wait(timeout=max(0,deadline-time.monotonic()))!=0:return {'state':'failed','error':'storage listing command failed'}
    except BaseException:
     process.kill();process.wait();raise
   lines=b''.join(chunks).decode('utf-8').splitlines()
  if not lines or lines[0].split()!=['Volid','Format','Type','Size','VMID']:raise ValueError('invalid storage listing header')
  volumes=[]
  for line in lines[1:]:
   if not line.strip():continue
   fields=line.split()
   # pvesm omits VMID for non-guest volumes; size may be unknown or approximate.
   if len(fields) not in (4,5) or not fields[0].startswith(storage_id+':') or not fields[0][len(storage_id)+1:]:raise ValueError('invalid storage listing row')
   if not re.fullmatch(r'(?:-|~?[0-9]+)',fields[3]) or (len(fields)==5 and not re.fullmatch(r'[0-9]+',fields[4])):raise ValueError('invalid storage listing row')
   volumes.append(fields[0])
  return {'state':'ok','volumes':sorted(set(volumes))}
 except subprocess.TimeoutExpired:return {'state':'failed','error':'storage listing timed out'}
 except FileNotFoundError:return {'state':'unknown','error':'storage listing command unavailable'}
 except (OSError,ValueError):return {'state':'failed','error':'storage listing could not be read completely'}

def read_recovery_files(root,paths,storage_ids,profile_paths):
 profile_paths=validate_host_profile(profile_paths)
 pbs=host_type(root)=='pbs'
 if pbs:
  profile_paths=pbs_capture_paths(profile_paths);data_roots=pbs_data_roots(root)
  if storage_ids:raise ValueError('PBS recovery does not list datastore contents')
 if not isinstance(paths,list) or not isinstance(storage_ids,list):raise ValueError('paths and storage IDs must be lists')
 for path in paths:
  if not isinstance(path,str) or len(path.encode())>4096:raise ValueError('invalid recovery path')
  relative(path)
  if pbs and (not path.startswith('etc/proxmox-backup/') or pbs_excluded(pathlib.Path(root)/path,data_roots)):raise ValueError('PBS recovery reads configuration only')
  # config.db decoding is an implicit part of collection even with a custom profile.
  if not any(path==p[1:] or path.startswith(p[1:]+'/') for p in profile_paths+['/etc/pve']):raise ValueError('recovery path outside host profile')
 for ident in storage_ids:
  if not isinstance(ident,str) or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_-]{0,63}',ident):raise ValueError('invalid storage ID')
 paths=list(dict.fromkeys(paths));unique_storage=list(dict.fromkeys(storage_ids));files=[]
 if len(paths)>RECOVERY_MAX_PATHS:files=[{'path':p,'state':'limit','error':'request exceeds 4096 unique paths'} for p in paths]
 else:
  root_fd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
  try:
   remaining=RECOVERY_MAX_TOTAL;metadata_remaining=RECOVERY_MAX_METADATA
   for path in paths:
    entry=read_recovery_file(root_fd,path,remaining)
    if entry['state']=='read':
     metadata_size=len(json.dumps(entry['metadata']).encode())
     if metadata_size>metadata_remaining:entry={'path':path,'state':'limit','error':'total metadata limit exceeded'}
     else:remaining-=entry['metadata']['size'];metadata_remaining-=metadata_size
    files.append(entry)
  finally:os.close(root_fd)
 if len(storage_ids)>RECOVERY_MAX_STORAGE_IDS:volumes={ident:{'state':'limit','error':'request exceeds 32 storage IDs'} for ident in unique_storage}
 else:
  deadline=time.monotonic()+RECOVERY_STORAGE_TIMEOUT
  volumes={ident:recovery_storage_volumes(ident,deadline) for ident in unique_storage}
 return {'files':files,'storage_volumes':volumes}

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
 inventory=probe(root,test_mode) if test_mode or host_type(root)=='pbs' else None
 platform=(inventory.get('host_type') or ('pbs' if inventory.get('pbs_version') else 'pve')) if inventory is not None else 'pve'
 data_roots=pbs_data_roots(root) if platform=='pbs' else []
 if platform=='pbs':paths=pbs_capture_paths(paths)
 entries=[];warnings=[];seen=set()
 def walk(p):
  rel=p.relative_to(root).as_posix()
  if platform=='pbs' and (pbs_excluded(p,data_roots) or '.chunks' in p.parts):return
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
  p=confined(root,source.lstrip('/')) if platform=='pbs' else root/source.lstrip('/')
  if p.exists() or p.is_symlink():walk(p)
  else:warnings.append('Required path missing: '+source)
 if platform=='pve':
  db=root/'var/lib/pve-cluster/config.db'
  try:
   (out/'recovery').mkdir(exist_ok=True)
   snapshot_db(db,out/'recovery/config.db')
   entries.extend(decode_db(out/'recovery/config.db',out))
  except (sqlite3.Error,OSError,ValueError) as exc:warnings.append('config.db capture failed: '+str(exc))
 if platform=='pbs':
  results=inventory.get('details',{}).get('command_results',{})
  for key in ('pbs_version','pbs_datastores','pbs_sync_jobs','pbs_prune_jobs','pbs_verify_jobs'):
   if results.get(key,{}).get('state')!='ok':warnings.append('PBS inventory query failed: '+key)
  if not any(e['path']=='etc/proxmox-backup' and e['type']=='directory' for e in entries):warnings.append('Required PBS configuration directory missing')
 if inventory is None:inventory=probe(root,test_mode)
 dependency_alerts,dependencies,scan_complete=dependency_scan(out,entries,root)
 warnings.extend(dependency_alerts)
 return {'inventory':inventory,'entries':entries,'warnings':warnings,
         'profile':{'version':1,'roots':list(paths),'dependency_entry_limit':10000,'dependency_file_limit':1024*1024,
                    'dependency_reference_limit':4096,'dependency_scan_complete':scan_complete},
         'dependencies':dependencies}

def dependency_warnings(out,entries,root=None):
 return dependency_scan(out,entries,root)[0]

def dependency_scan(out,entries,root=None):
 """Bounded discovery of Proxmox hooks and explicit config/secret file references."""
 known={e['path'] for e in entries};warnings=[];storage={};dependencies=[];complete=True
 def note(candidate,source,state,reason):
  nonlocal complete
  if len(dependencies)>=4096:
   complete=False
   return
  dependencies.append({'path':candidate,'source':source,'state':state,'reason':reason})
 p=out/'files/etc/pve/storage.cfg'
 if p.exists():
  if p.stat().st_size>1024*1024:complete=False
  else:
   current=None
   for line in p.read_text(errors='replace').splitlines():
    match=re.match(r'^dir:\s*(\S+)',line)
    if match:current=match.group(1)
    elif line and not line[0].isspace():current=None
    elif current:
     match=re.match(r'^\s+path\s+(\S+)',line)
     if match:storage[current]=match.group(1)
 for e in entries[:10000]:
  if e.get('type')!='file' or not e['path'].startswith('etc/'):continue
  if e.get('size',0)>1024*1024:complete=False;continue
  p=out/'files'/e['path']
  try:data=p.read_text(errors='replace')
  except OSError:complete=False;continue
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
     note(candidate,e['path'],'captured' if candidate.lstrip('/') in known else 'missing','captured' if candidate.lstrip('/') in known else 'outside_capture')
     if candidate.lstrip('/') not in known:warnings.append('Referenced hookscript is not captured: '+candidate+' ('+e['path']+')')
    else:
     note(ref,e['path'],'unknown','unresolved_reference')
     warnings.append('Unresolved hookscript dependency: '+ref+' ('+e['path']+')')
   match=re.search(r'(?i)\b(keyfile|key_file|ssl_keyfile|privatekeyfile|certificatefile|certfile|ca_file|credentials|secret_file|include|source|script)\s*(?:[:=]\s*|\s+)["\']?(/[^\s"\';]+)',line)
   if match:
    directive=match.group(1).lower();candidate=match.group(2)
    if '*' in candidate or '?' in candidate:continue
    # Powerman device scripts contain remote ILOM commands such as
    # send "start -script /SYS\\n", not local script filenames.
    if re.fullmatch(r'etc/powerman/[^/]+\.dev',e['path']) and directive=='script' and re.match(r'^send\s+"',line) and line.endswith('"') and '-script '+candidate in line:continue
    # Debian's Lynx configuration names a local override even when none exists.
    # Only proven absence is optional; unreadable files and broken links warn.
    if e['path']=='etc/lynx/lynx.cfg' and directive=='include' and candidate=='/etc/lynx/local.cfg' and re.fullmatch(r'(?i)INCLUDE:/etc/lynx/local\.cfg\s*',line) and root is not None:
     try:(root/'etc/lynx/local.cfg').lstat()
     except FileNotFoundError:continue
     except OSError:pass
    # RNG source selects a runtime device, not a restorable config file.
    if vm_config and directive=='source' and re.fullmatch(r'rng0\s*:\s*source=/dev/(?:urandom|random|hwrng)(?:,(?:max_bytes|period)=\d+)*',line):continue
    # Only absence proven on the source host makes a guarded Vim source optional.
    # lstat preserves warnings for broken links and never reads device contents.
    if vim_config and directive=='source' and re.match(r'^source\s+',line) and '|' not in line and candidate in vim_guards and root is not None:
     try:(root/candidate.lstrip('/')).lstat()
     except FileNotFoundError:continue
     except OSError:pass
    note(candidate,e['path'],'captured' if candidate.lstrip('/') in known else 'missing','captured' if candidate.lstrip('/') in known else 'outside_capture')
    if candidate.lstrip('/') not in known:warnings.append('Referenced config/secret is not captured: '+candidate+' ('+e['path']+')')
 if len(entries)>10000:complete=False
 if not complete:warnings.append('Dependency scan incomplete: bounded discovery limit reached')
 return sorted(set(warnings)),dependencies,complete

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
 if path.startswith('etc/proxmox-backup/') and not pbs_restore_path(path):return True
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
    if not re.fullmatch(r'[0-9a-f]{64}',e[key]) and not (e[key]=='missing' and (key=='before_sha' or (e.get('action')=='remove' and e.get('after_metadata') is None and e.get('before_sha')!='missing'))):raise ValueError('invalid journal hash')
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
 pbs_journal_guard(root,j)
 j['state']='rolling_back';save_journal(folder,j);conflict=False
 for e in reversed(j['files']):
  if e['phase'] not in ('writing','applied','rolling_back') and not (retry_conflicts and e['phase']=='rollback_conflict'):continue
  try:
   p=confined(root,e['path']);current=file_state(root,e['path'],test_mode)
   if current==(e['before_sha'],e['before_metadata']):e['phase']='rolled_back';save_journal(folder,j);continue
   if current!=(e['after_sha'],e['after_metadata']):
    e['phase']='rollback_conflict';conflict=True;save_journal(folder,j);continue
   pbs_journal_guard(root,j)
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
 if host_type(root)=='pbs':pbs_quiescent_identity(root)
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
 if host_type(root)=='pbs':pbs_quiescent_identity(root)
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
  pbs=host_type(root)=='pbs'
  pbs_identity=pbs_quiescent_identity(root) if pbs else None
  prepared=[];seen=set()
  for item in items:
   rel=item['path'];p=confined(root,rel)
   if pbs:
    if not pbs_restore_path(rel) or item.get('action','replace')!='replace' or pbs_excluded(p,pbs_data_roots(root)):raise ValueError('PBS recovery writes configuration files only; no deletions')
   elif rel.startswith('etc/proxmox-backup/'):raise ValueError('PBS configuration requires a PBS target')
   if protected_restore(rel):raise ValueError('protected restore requires manual procedure: '+rel)
   if rel in seen:raise ValueError('duplicate restore path')
   seen.add(rel)
   if item.get('type','file')!='file' or item.get('xattrs'):raise ValueError('file metadata requires manual procedure')
   action=item.get('action','replace')
   if action not in ('replace','remove'):raise ValueError('unknown restore action')
   if action=='remove' and (item.get('remove_confirmed') is not True or not rel.startswith('etc/') or any(k in item for k in ('content','mode','uid','gid','user','group'))):raise ValueError('explicit removal confirmation and deletion-only fields required')
   before,meta=file_state(root,rel,test_mode)
   if item.get('before_sha')!=before:raise ValueError('target file changed: '+rel)
   if not test_mode and 'expected_metadata' not in item:raise ValueError('expected target metadata required')
   expected=item.get('expected_metadata',meta)
   if expected!=meta:raise ValueError('target metadata changed: '+rel)
   if meta and meta['xattrs']:raise ValueError('target ACL/xattrs require manual procedure')
   if action=='remove':
    if before=='missing' or meta is None:raise ValueError('removal target must exist')
    prepared.append((p,item,None,meta,before));continue
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
   if pbs:validate_pbs_restore_content(root,rel,data)
   prepared.append((p,item,data,meta,before))
  if not prepared:raise ValueError('no restore files')
  folder.mkdir(parents=True,mode=0o700);sync_ancestors(root,folder)
  j={'version':2,'operation_id':plan_id,'state':'writing','files':[],'checks':['helper lock held; external writers require an operator change freeze','service and reboot checks require operator']}
  if pbs:j.update(platform='pbs',pbs_datastore_identity=pbs_identity)
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
    if item.get('action')=='remove':
     j['files'].append({'path':rel,'action':'remove','before_sha':before,'before_metadata':meta,'after_sha':'missing','after_metadata':None,'stage':None,'phase':'prepared','applied':False})
     continue
    stage=write_file(p,data,item.get('mode',0o600),item.get('uid',0),item.get('gid',0),test_mode);stages.append(stage)
    after_meta=metadata(pathlib.Path(stage),test_mode)
    j['files'].append({'path':rel,'before_sha':before,'before_metadata':meta,'after_sha':digest(data),'after_metadata':after_meta,'stage':str(stage),'phase':'prepared','applied':False})
   # Every rollback file, staging file and metadata record is durable before writes.
   for directory,_,_ in os.walk(folder,topdown=False):sync_directory(pathlib.Path(directory))
   save_journal(folder,j)
   for e in j['files']:
    pbs_journal_guard(root,j)
    p=confined(root,e['path'])
    e['phase']='writing';save_journal(folder,j)
    if file_state(root,e['path'],test_mode)!=(e['before_sha'],e['before_metadata']):raise ValueError('target changed before replace: '+e['path'])
    if e.get('action')=='remove':
     # The original bytes and metadata are durable before the journal authorizes unlink.
     if file_state(root,e['path'],test_mode)!=(e['before_sha'],e['before_metadata']):raise ValueError('target changed before removal: '+e['path'])
     p.unlink();e['applied']=True;sync_directory(p.parent)
     if file_state(root,e['path'],test_mode)!=('missing',None):raise ValueError('removal verification failed: '+e['path'])
     e['phase']='applied';save_journal(folder,j);continue
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

VERIFY_SERVICES=('pve-cluster','pvedaemon','pveproxy')+PBS_SERVICES
def verification_identity(root):
 try:
  with (root/'etc/machine-id').open('rb') as f:raw=f.read(129).strip()
  if not re.fullmatch(b'[0-9a-f]{32}',raw) or raw==b'0'*32:raise ValueError('invalid machine identity')
  return {'state':'ok','value':digest(raw)}
 except (OSError,ValueError):return {'state':'unknown','value':''}
def verification_boot(root):
 try:
  with (root/'proc/sys/kernel/random/boot_id').open('rb') as f:raw=f.read(129).strip().decode('ascii')
  if not re.fullmatch(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}',raw):raise ValueError('invalid boot ID')
  return {'state':'ok','value':raw}
 except (OSError,ValueError):return {'state':'unknown','value':''}
def verify_service(name):
 if name not in VERIFY_SERVICES:raise ValueError('invalid service')
 # No captured command text or unbounded subprocess pipes: systemctl output is discarded.
 try:
  p=subprocess.run(['systemctl','is-active','--quiet',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10,check=False)
  return {'state':'ok','value':'active'} if p.returncode==0 else {'state':'failed','value':''}
 except FileNotFoundError:return {'state':'unknown','value':''}
 except (OSError,subprocess.TimeoutExpired):return {'state':'failed','value':''}
def verify_recovery(root,services,storage_ids):
 if host_type(root)=='pbs' and (storage_ids or any(name not in PBS_SERVICES for name in services)):raise ValueError('PBS verification reads service state only; no datastore contents')
 if not isinstance(services,list) or not isinstance(storage_ids,list) or len(services)>3 or len(storage_ids)>32:raise ValueError('invalid verification selection')
 if any(not isinstance(v,str) or v not in VERIFY_SERVICES for v in services) or len(set(services))!=len(services):raise ValueError('invalid service selection')
 if any(not isinstance(v,str) or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_-]{0,63}',v) for v in storage_ids) or len(set(storage_ids))!=len(storage_ids):raise ValueError('invalid storage selection')
 identity=verification_identity(root);boot=verification_boot(root)
 deadline=time.monotonic()+RECOVERY_STORAGE_TIMEOUT
 volumes={ident:recovery_storage_volumes(ident,deadline) for ident in storage_ids}
 for ident,v in volumes.items():
  if len(v.get('volumes',[]))>10000 or any(len(name)>256 for name in v.get('volumes',[])):volumes[ident]={'state':'limit','error':'volume limit exceeded'}
 result={'protocol':1,'observed_at':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'identity':identity,'boot':boot,'services':{name:verify_service(name) for name in services},'storage_volumes':volumes}
 if verification_identity(root)!=identity or verification_boot(root)!=boot:raise ValueError('machine changed during verification')
 if len(json.dumps(result).encode())>256*1024:raise ValueError('verification output limit exceeded')
 return result

def discovery_hints(root):
 """Read configured peers only. No network contact to peers, passwords or file inventory."""
 root=pathlib.Path(root);hints=[];seen=set();partial=False
 def add(value,name,source):
  nonlocal partial
  try:
   address=ipaddress.ip_address(value)
   if address.is_loopback or address.is_link_local or address.is_multicast or address.is_unspecified:raise ValueError('non-unicast')
  except ValueError:partial=True;return
  key=str(address)
  if key not in seen and len(hints)<512:
   hints.append({'address':key,'name':str(name)[:256],'source':source});seen.add(key)
 if host_type(root)=='pbs':
  for item in pbs_sections(root,'remote.cfg','remote',{'host'}):
   if item.get('host'):add(item['host'],item['name'],'pbs-config')
  return {'version':1,'state':'partial' if partial else 'ok','hints':hints}
 config=confined(root,'etc/pve/corosync.conf')
 if not config.exists():return {'version':1,'state':'not_applicable','hints':[]}
 if config.stat().st_size>1024*1024:raise ValueError('cluster configuration exceeds limit')
 raw=command(['pvesh','get','/cluster/status','--output-format','json'])
 rows=asjson(raw,None)
 if isinstance(rows,list):
  for row in rows[:512]:
   if isinstance(row,dict) and row.get('type')=='node' and isinstance(row.get('ip'),str):add(row['ip'],row.get('name',''),'cluster')
 else:partial=True
 # Corosync contains explicitly configured ring addresses, not necessarily
 # management addresses. The central scanner checks scope and service identity.
 node=None;depth=0;node_depth=None
 for line in config.read_text().splitlines():
  line=line.split('#',1)[0].strip()
  if re.match(r'^node\s*\{',line):node={};node_depth=depth+1
  if node is not None:
   match=re.fullmatch(r'(name|ring[01]_addr):\s*(\S+)',line)
   if match:node[match.group(1)]=match.group(2)
  depth+=line.count('{')-line.count('}')
  if node is not None and depth<node_depth:
   for field in ('ring0_addr','ring1_addr'):
    if field in node:add(node[field],node.get('name',''),'cluster')
   node=None;node_depth=None
 return {'version':1,'state':'partial' if partial else 'ok','hints':hints}

def authorize(operation,read_only=False):
 if operation not in ("probe","discover","collect","read-recovery","verify-recovery","apply","restore-status","rollback"):raise ValueError("unknown operation")
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
 if op=='discover':
  if set(request)!={'version','operation'}:raise ValueError('invalid discovery request')
  print(json.dumps(discovery_hints(root)))
 elif op=='probe':print(json.dumps(probe()))
 elif op=='collect':
  paths=load_host_profile(root)
  with tempfile.TemporaryDirectory(prefix='anker-host-') as d:
   out=pathlib.Path(d);c=collect(root,out,paths);(out/'collection.json').write_text(json.dumps(c))
   with tarfile.open(fileobj=sys.stdout.buffer,mode='w|') as t:
    for p in sorted(out.rglob('*')):t.add(p,arcname=p.relative_to(out).as_posix(),recursive=False)
 elif op=='read-recovery':print(json.dumps(read_recovery_files(root,request.get('paths',[]),request.get('storage_ids',[]),load_host_profile(root))))
 elif op=='verify-recovery':
  if set(request)!={'version','operation','services','storage_ids'}:raise ValueError('invalid verification request fields')
  print(json.dumps(verify_recovery(root,request['services'],request['storage_ids'])))
 elif op=='restore-status':print(json.dumps(restore_status(root,request['plan_id'])))
 elif op=='rollback':print(json.dumps(rollback_files(root,request['plan_id'],request.get('confirm'))))
 elif op=='apply':
  if not request.get('confirm') or request.get('confirm')!=request.get('plan_id'):raise ValueError('explicit plan confirmation required')
  operation_id(request.get('plan_id'))
  if any('expected_metadata' not in e for e in request.get('files',[])):raise ValueError('expected target metadata required')
  expected=dict(request['expected_inventory']);current=probe(root)
  required=(('pbs_version','pbs_services','pbs_datastore_identity') if current.get('host_type')=='pbs' else ('pve_version',))+('users','groups','file_inventory')
  for inv in (expected,current):
   details=inv.get('details',{})
   if details.get('capabilities',{}).get('restore_protocol')!=2 or details.get('capabilities',{}).get('journal') is not True:raise ValueError('hardened restore capabilities required')
   if any(details.get('command_results',{}).get(k,{}).get('state')!='ok' for k in required):raise ValueError('required target inventory query failed')
  for x in ('captured_at','fingerprint'):expected.pop(x,None);current.pop(x,None)
  stable=('file_hashes','file_metadata','users','groups','capabilities','command_results','boot_id','addresses','routes','packages','manual_packages','pci','boot','pbs_services','pbs_datastore_identity')
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
