import base64, hashlib, importlib.util, io, json, os, pathlib, subprocess, tempfile, unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('recovery_helper',pathlib.Path(__file__).with_name('anker_host.py'))
h=importlib.util.module_from_spec(spec);spec.loader.exec_module(h)

class RecoveryReadTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=pathlib.Path(self.tmp.name)
  (self.root/'etc').mkdir();self.a=self.root/'etc/a';self.a.write_bytes(b'configuration');self.a.chmod(0o640)
  # The macOS test runtime lacks xattr APIs; Linux production uses descriptor APIs.
  if not hasattr(os,'listxattr'):
   self.addCleanup(patch.stopall);patch.object(h.os,'listxattr',return_value=[],create=True).start();patch.object(h.os,'getxattr',return_value=b'',create=True).start()
 def read(self,paths,storage_ids=None,profile=None):
  self.assertTrue(callable(getattr(h,'read_recovery_files',None)),'bounded recovery read operation is missing')
  return h.read_recovery_files(self.root,paths,storage_ids or [],profile or ['/etc','/usr/local'])
 def request(self,value,read_only=True):
  self.assertTrue(callable(getattr(h,'read_recovery_files',None)),'bounded recovery read operation is missing')
  stdin=type('Stdin',(),{'buffer':io.BytesIO(json.dumps(value).encode()+b'\n')})();stdout=io.StringIO()
  with patch.object(h.os,'geteuid',return_value=0),patch.object(h.sys,'platform','linux'),patch.object(h.sys,'argv',['anker-host']+(['--read-only'] if read_only else [])),patch.object(h.sys,'stdin',stdin),patch.object(h.sys,'stdout',stdout):h.main(root=self.root)
  return json.loads(stdout.getvalue())
 def test_read_authority_reads_without_mutation(self):
  before={p.relative_to(self.root).as_posix():(p.read_bytes(),p.stat().st_mode,p.stat().st_mtime_ns) for p in self.root.rglob('*') if p.is_file()}
  result=self.request({'version':1,'operation':'read-recovery','paths':['etc/a'],'storage_ids':[]})
  entry=result['files'][0];self.assertEqual(entry['state'],'read');self.assertEqual(base64.b64decode(entry['content']),b'configuration')
  self.assertEqual(entry['sha256'],hashlib.sha256(b'configuration').hexdigest());self.assertEqual(entry['metadata']['mode'],0o640);self.assertEqual(entry['metadata']['path'],'etc/a')
  after={p.relative_to(self.root).as_posix():(p.read_bytes(),p.stat().st_mode,p.stat().st_mtime_ns) for p in self.root.rglob('*') if p.is_file()}
  self.assertEqual(after,before);self.assertFalse((self.root/'var/lib/anker-host').exists());self.assertEqual(result['storage_volumes'],{})
  for operation in ('apply','restore-status','rollback'):
   with self.assertRaisesRegex(ValueError,'read-only'):h.authorize(operation,True)
 def test_symlink_swap_never_exposes_external_content(self):
  outside=self.root/'outside';outside.mkdir();(outside/'a').write_bytes(b'external-secret')
  real_open=h.os.open;swapped=False
  def open_path(path,flags,*args,**kwargs):
   nonlocal swapped
   if path=='etc' and kwargs.get('dir_fd') is not None and not swapped:
    (self.root/'etc').rename(self.root/'original');(self.root/'etc').symlink_to(outside);swapped=True
   return real_open(path,flags,*args,**kwargs)
  with patch.object(h.os,'open',side_effect=open_path):entry=self.read(['etc/a'])['files'][0]
  self.assertNotEqual(entry['state'],'read');self.assertNotIn('content',entry);self.assertNotIn('external-secret',json.dumps(entry))
 def test_missing_unreadable_and_empty_are_distinct(self):
  (self.root/'etc/empty').write_bytes(b'');real_open=h.os.open
  def open_path(path,flags,*args,**kwargs):
   if path=='a':raise PermissionError('denied')
   return real_open(path,flags,*args,**kwargs)
  with patch.object(h.os,'open',side_effect=open_path):result=self.read(['etc/absent','etc/a','etc/empty'])
  missing,unreadable,empty=result['files'];self.assertEqual(missing['state'],'missing');self.assertEqual(unreadable['state'],'unreadable');self.assertEqual(empty['state'],'read')
  self.assertNotIn('content',missing);self.assertNotIn('content',unreadable);self.assertEqual(empty['content'],'');self.assertEqual(empty['sha256'],'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
 def test_profile_boundary_and_request_limits(self):
  for path in ('etc/ab/private','opt/secret','../escape','/etc/a','etc//a','etc/./a'):
   with self.subTest(path=path),self.assertRaises(ValueError):self.read([path],profile=['/etc/a'])
  self.assertEqual(len(self.read(['etc/a']*4097)['files']),1)
  result=self.read(['etc/missing-'+str(i) for i in range(4097)])
  self.assertEqual(len(result['files']),4097);self.assertTrue(all(e['state']=='limit' and 'content' not in e for e in result['files']))
  result=self.read([],['store'+str(i) for i in range(33)])
  self.assertEqual(len(result['storage_volumes']),33);self.assertTrue(all(e['state']=='limit' for e in result['storage_volumes'].values()))
  # Collection always decodes config.db under /etc/pve, including custom profiles.
  p=self.root/'etc/pve/storage.cfg';p.parent.mkdir();p.write_bytes(b'dir: local\n')
  self.assertEqual(self.read(['etc/pve/storage.cfg'],profile=['/etc/a'])['files'][0]['state'],'read')
 def test_storage_listing_failure_is_not_empty(self):
  # Real subprocesses simulate bounded pvesm output without reaching a host.
  real_popen=subprocess.Popen
  def listing(args,**kwargs):
   self.assertEqual(args,['pvesm','list','local']);self.assertNotIn('shell',kwargs)
   return real_popen([h.sys.executable,'-c','import sys; sys.exit(3)'],**kwargs)
  with patch.object(h.subprocess,'Popen',side_effect=listing):result=self.read([],['local'])
  self.assertEqual(result['storage_volumes']['local']['state'],'failed');self.assertNotIn('volumes',result['storage_volumes']['local'])
 def test_regular_file_and_aggregate_size_limits_never_return_partial_content(self):
  (self.root/'etc/large').write_bytes(b'x'*(1024*1024+1))
  entry=self.read(['etc/large'])['files'][0];self.assertEqual(entry['state'],'too_large');self.assertNotIn('content',entry)
  paths=[]
  for i in range(9):
   path='etc/chunk-'+str(i);(self.root/path).write_bytes(b'x'*(1024*1024));paths.append(path)
  result=self.read(paths)['files'];self.assertEqual([e['state'] for e in result],['read']*8+['limit']);self.assertNotIn('content',result[-1])
 def test_special_files_and_final_symlink_are_unsupported(self):
  (self.root/'etc/link').symlink_to(self.a);(self.root/'etc/folder').mkdir();os.mkfifo(self.root/'etc/pipe')
  result=self.read(['etc/link','etc/folder','etc/pipe'])['files'];self.assertTrue(all(e['state']=='unsupported' and 'content' not in e for e in result))
 def test_content_or_metadata_change_during_read_discards_content(self):
  real_read=h.os.read;changed=False
  def read(fd,size):
   nonlocal changed
   data=real_read(fd,size)
   if data and not changed:self.a.write_bytes(b'admin-edit');self.a.chmod(0o600);changed=True
   return data
  with patch.object(h.os,'read',side_effect=read):entry=self.read(['etc/a'])['files'][0]
  self.assertEqual(entry['state'],'changed');self.assertNotIn('content',entry)
 def test_metadata_failure_is_unreadable(self):
  with patch.object(h.os,'listxattr',side_effect=PermissionError('metadata denied'),create=True):entry=self.read(['etc/a'])['files'][0]
  self.assertEqual(entry['state'],'unreadable');self.assertNotIn('content',entry)
 def test_storage_success_empty_and_volume_inventory(self):
  real_popen=subprocess.Popen
  for output,volumes in [('Volid Format Type Size VMID\n',[]),('Volid Format Type Size VMID\nlocal:100/vm-100-disk-0.qcow2 qcow2 images 123 100\n',['local:100/vm-100-disk-0.qcow2'])]:
   with self.subTest(output=output):
    with patch.object(h.subprocess,'Popen',side_effect=lambda args,**kw:real_popen([h.sys.executable,'-c','print('+repr(output)+',end="")'],**kw)):
     result=self.read([],['local'])['storage_volumes']['local']
    self.assertEqual(result,{'state':'ok','volumes':volumes})
 def test_storage_output_overflow_timeout_and_invalid_ids(self):
  real_popen=subprocess.Popen
  for source,state in [('import sys; sys.stdout.write("x"*(1024*1024+1))','limit'),('import time; time.sleep(2)','failed'),('print("unexpected inventory")','failed')]:
   with self.subTest(state=state),patch.object(h,'RECOVERY_STORAGE_TIMEOUT',0.1,create=True),patch.object(h.subprocess,'Popen',side_effect=lambda args,**kw:real_popen([h.sys.executable,'-c',source],**kw)):
    entry=self.read([],['local'])['storage_volumes']['local'];self.assertEqual(entry['state'],state);self.assertNotIn('volumes',entry)
  for ident in ('-x','local;touch /tmp/x','local/x','', 'a'*65):
   with self.subTest(ident=ident),self.assertRaises(ValueError):self.read([],[ident])
 def test_profile_permissions_and_symlinks_are_rejected(self):
  config=self.root/'etc/anker-host.json';config.write_text('{"paths":["/etc/a"]}');config.chmod(0o666)
  with self.assertRaisesRegex(ValueError,'root-owned'):self.request({'version':1,'operation':'read-recovery','paths':['etc/a']})
  config.unlink();config.symlink_to(self.a)
  with self.assertRaises((ValueError,OSError)):self.request({'version':1,'operation':'read-recovery','paths':['etc/a']})

 def test_failure_after_open_is_not_reported_as_missing(self):
  with patch.object(h.os,'read',side_effect=FileNotFoundError('read failed')):entry=self.read(['etc/a'])['files'][0]
  self.assertEqual(entry['state'],'unreadable');self.assertNotIn('content',entry)
 def test_content_not_exposed_when_metadata_budget_is_exceeded(self):
  with patch.object(h,'RECOVERY_MAX_METADATA',1,create=True):entry=self.read(['etc/a'])['files'][0]
  self.assertEqual(entry['state'],'limit');self.assertNotIn('content',entry)
 def test_root_owned_custom_profile_is_used_by_main(self):
  config=self.root/'etc/anker-host.json';config.write_text('{"paths":["/etc/a"]}');config.chmod(0o600)
  real_fstat=h.os.fstat;inode=config.stat().st_ino
  def fstat(fd):
   st=real_fstat(fd)
   if st.st_ino==inode:
    values=list(st);values[4]=0;return os.stat_result(values)
   return st
  with patch.object(h.os,'fstat',side_effect=fstat):
   self.assertEqual(self.request({'version':1,'operation':'read-recovery','paths':['etc/a']})['files'][0]['state'],'read')
   with self.assertRaisesRegex(ValueError,'profile'):self.request({'version':1,'operation':'read-recovery','paths':['etc/other']})
 def test_missing_storage_command_remains_unknown(self):
  with patch.object(h.subprocess,'Popen',side_effect=FileNotFoundError('absent')):entry=self.read([],['local'])['storage_volumes']['local']
  self.assertEqual(entry['state'],'unknown');self.assertNotIn('volumes',entry)

 def test_profile_trailing_separators_match_collection_paths(self):
  self.assertEqual(self.read(['etc/a'],profile=['/etc/'])['files'][0]['state'],'read')
  with self.assertRaises(ValueError):self.read(['proc/file'],profile=['/proc/'])
 def test_extended_metadata_limit_is_explicit(self):
  with patch.object(h.os,'listxattr',return_value=['user.large'],create=True),patch.object(h.os,'getxattr',return_value=b'x'*(64*1024+1),create=True):entry=self.read(['etc/a'])['files'][0]
  self.assertEqual(entry['state'],'limit');self.assertNotIn('content',entry)

 def test_storage_official_optional_vmid_and_size_forms(self):
  real_popen=subprocess.Popen
  fixtures=[
   ('local:iso/debian.iso iso iso 123', ['local:iso/debian.iso']),
   ('local:100/vm-100-disk-0.qcow2 qcow2 images - 100', ['local:100/vm-100-disk-0.qcow2']),
   ('local:100/vm-100-disk-0.qcow2 qcow2 images ~123 100', ['local:100/vm-100-disk-0.qcow2']),
   ('local:100/vm-100-disk-0.qcow2 qcow2 images 123 100\nlocal:iso/debian.iso iso iso -\nlocal:vztmpl/debian.tar.zst tgz vztmpl ~456', ['local:100/vm-100-disk-0.qcow2','local:iso/debian.iso','local:vztmpl/debian.tar.zst']),
  ]
  for rows,volumes in fixtures:
   output='Volid Format Type Size VMID\n'+rows+'\n'
   with self.subTest(rows=rows),patch.object(h.subprocess,'Popen',side_effect=lambda args,**kw:real_popen([h.sys.executable,'-c','print('+repr(output)+',end="")'],**kw)):
    self.assertEqual(self.read([],['local'])['storage_volumes']['local'],{'state':'ok','volumes':volumes})
 def test_storage_malformed_row_discards_complete_listing(self):
  real_popen=subprocess.Popen
  for row in ('other:iso/debian.iso iso iso 123','local: iso iso 123','local:iso/debian.iso iso iso','local:iso/debian.iso iso iso ~','local:iso/debian.iso iso iso -123','local:iso/debian.iso iso iso ~1x','local:iso/debian.iso iso iso 123 guest','local:iso/debian.iso iso iso 123 100 extra'):
   output='Volid Format Type Size VMID\nlocal:100/vm-100-disk-0.qcow2 qcow2 images 123 100\n'+row+'\n'
   with self.subTest(row=row),patch.object(h.subprocess,'Popen',side_effect=lambda args,**kw:real_popen([h.sys.executable,'-c','print('+repr(output)+',end="")'],**kw)):
    entry=self.read([],['local'])['storage_volumes']['local'];self.assertEqual(entry['state'],'failed');self.assertNotIn('volumes',entry)

if __name__=='__main__':unittest.main()
