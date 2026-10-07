import unittest,tempfile,pathlib,sqlite3,importlib.util,json
spec=importlib.util.spec_from_file_location('helper',pathlib.Path(__file__).with_name('anker_host.py'))
helper=importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
class HostHelperTests(unittest.TestCase):
 def test_snapshot_matches_database_tree(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();(root/'var/lib/pve-cluster').mkdir(parents=True)
   db=sqlite3.connect(root/'var/lib/pve-cluster/config.db')
   db.execute('CREATE TABLE tree(inode INTEGER PRIMARY KEY,parent INTEGER,version INTEGER,writer INTEGER,mtime INTEGER,type INTEGER,name TEXT,data BLOB)')
   db.execute('INSERT INTO tree VALUES(0,0,1,0,1,4,?,NULL)',('',))
   db.execute('INSERT INTO tree VALUES(1,0,1,0,1,8,?,?)',('storage.cfg',b'dir: local\n path /var/lib/vz\n'));db.commit();db.close()
   out=root/'out';out.mkdir();result=helper.collect(root,out,['/etc'],test_mode=True)
   self.assertEqual((out/'files/etc/pve/storage.cfg').read_bytes(),b'dir: local\n path /var/lib/vz\n')
   self.assertTrue((out/'recovery/config.db').is_file())
 def test_symlink_is_not_followed(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();(r/'etc/escape').symlink_to('/etc/passwd');out=r/'out';out.mkdir()
   c=helper.collect(r,out,['/etc'],test_mode=True)
   e=next(e for e in c['entries'] if e['path']=='etc/escape')
   self.assertEqual(e['type'],'symlink');self.assertFalse((out/'files/etc/escape').is_symlink())
 def test_missing_pve_is_partial(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();out=r/'out';out.mkdir()
   c=helper.collect(r,out,['/etc'],test_mode=True)
   self.assertTrue(any('config.db' in w for w in c['warnings']))
 def test_apply_rejects_drift_before_write(self):
  with tempfile.TemporaryDirectory() as d:
   r=pathlib.Path(d);(r/'etc').mkdir();(r/'etc/test').write_text('changed')
   with self.assertRaises(ValueError):helper.apply_files(r,[{'path':'etc/test','before_sha':'wrong','content':'bmV3','mode':420,'uid':0,'gid':0}],test_mode=True)
   self.assertEqual((r/'etc/test').read_text(),'changed')
 def test_traversal_rejected(self):
  with tempfile.TemporaryDirectory() as d:
   with self.assertRaises(ValueError):helper.apply_files(pathlib.Path(d),[{'path':'../escape','content':'eA=='}],test_mode=True)
if __name__=='__main__':unittest.main()

class BackupAuthorityTests(unittest.TestCase):
 def test_read_only_channel_rejects_apply(self):
  with self.assertRaises(ValueError):helper.authorize('apply',read_only=True)
  helper.authorize('probe',read_only=True);helper.authorize('collect',read_only=True)

class CompletenessTests(unittest.TestCase):
 def test_metadata_read_failure_is_not_silent(self):
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d)/'config';p.write_text('test')
   with patch.object(helper.os,'listxattr',side_effect=PermissionError('metadata denied'),create=True):
    self.assertIn('metadata_warning',helper.entry_meta(p,'etc/config'))
 def test_external_secret_dependency_is_reported(self):
  with tempfile.TemporaryDirectory() as d:
   out=pathlib.Path(d);(out/'files/etc').mkdir(parents=True);(out/'files/etc/app.conf').write_text('keyfile = /opt/secrets/app.key\n')
   warnings=helper.dependency_warnings(out,[{'path':'etc/app.conf','type':'file','size':40}])
   self.assertTrue(any('/opt/secrets/app.key' in w for w in warnings))
 def test_proxmox_hookscript_dependency_is_reported(self):
  with tempfile.TemporaryDirectory() as d:
   out=pathlib.Path(d);(out/'files/etc/pve/qemu-server').mkdir(parents=True)
   (out/'files/etc/pve/storage.cfg').write_text('dir: local\n    path /var/lib/vz\n')
   (out/'files/etc/pve/qemu-server/100.conf').write_text('hookscript: local:snippets/hook.sh\n')
   warnings=helper.dependency_warnings(out,[{'path':'etc/pve/qemu-server/100.conf','type':'file','size':40}])
   self.assertTrue(any('/var/lib/vz/snippets/hook.sh' in w for w in warnings))

 def collect_config(self,root,path,content,paths=None):
  config=root/path;config.parent.mkdir(parents=True,exist_ok=True);config.write_text(content)
  dbpath=root/'var/lib/pve-cluster/config.db';dbpath.parent.mkdir(parents=True,exist_ok=True)
  with sqlite3.connect(dbpath) as db:
   db.execute('CREATE TABLE tree(inode INTEGER PRIMARY KEY,parent INTEGER,mtime INTEGER,type INTEGER,name TEXT,data BLOB)')
  out=root/'out';out.mkdir()
  warnings=helper.collect(root,out,paths or ['/etc'],test_mode=True)['warnings']
  return [warning for warning in warnings if warning.startswith(('Referenced ','Unresolved '))]

 def test_proxmox_rng_device_is_not_a_missing_file_dependency(self):
  for path in ('etc/pve/qemu-server/9901.conf','etc/pve/nodes/zp01/qemu-server/9901.conf'):
   for device in ('/dev/urandom','/dev/random','/dev/hwrng'):
    for options in ('',',max_bytes=1024,period=1000'):
     with self.subTest(path=path,device=device,options=options),tempfile.TemporaryDirectory() as d:
      out=pathlib.Path(d);config=out/'files'/path;config.parent.mkdir(parents=True);config.write_text('rng0: source='+device+options+'\n')
      self.assertEqual(helper.dependency_warnings(out,[{'path':path,'type':'file','size':config.stat().st_size}]),[])

 def test_rng_exception_preserves_other_file_dependencies(self):
  cases=(('etc/app.conf','source=/dev/urandom\n','/dev/urandom'),
         ('etc/pve/nodes/zp01/qemu-server/9901.conf','rng0: source=/opt/secrets/random.seed\n','/opt/secrets/random.seed'),
         ('etc/pve/nodes/zp01/qemu-server/9901.conf','keyfile=/dev/urandom\n','/dev/urandom'),
         ('etc/pve/nodes/zp01/qemu-server/9901.conf','rng0: source=/dev/urandom,keyfile=/opt/secrets/key\n','/opt/secrets/key'),
         ('etc/pve/nodes/zp01/qemu-server/9901.conf','rng0: source=/dev/urandom\ncredentials=/opt/secrets/auth\ninclude /opt/config\n','/opt/secrets/auth'))
  for path,content,missing in cases:
   with self.subTest(path=path,content=content),tempfile.TemporaryDirectory() as d:
    out=pathlib.Path(d);config=out/'files'/path;config.parent.mkdir(parents=True);config.write_text(content)
    warnings=helper.dependency_warnings(out,[{'path':path,'type':'file','size':len(content)}])
    self.assertTrue(any(missing in warning for warning in warnings))

 def test_absent_vim_source_guarded_by_filereadable_is_optional(self):
  for quote in ('"',"'"):
   with self.subTest(quote=quote),tempfile.TemporaryDirectory() as d:
    content='if filereadable('+quote+'/etc/vim/vimrc.local'+quote+')\n  source /etc/vim/vimrc.local\nendif\n'
    self.assertEqual(self.collect_config(pathlib.Path(d),'etc/vim/vimrc',content),[])

 def test_existing_optional_vim_source_not_captured_still_warns(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);target=root/'etc/vim/vimrc.local';target.parent.mkdir(parents=True);target.write_text('set number\n')
   warnings=self.collect_config(root,'etc/vim/vimrc','if filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local\nendif\n',paths=['/etc/vim/vimrc'])
   self.assertTrue(any('/etc/vim/vimrc.local' in warning for warning in warnings))

 def test_vim_guard_does_not_suppress_required_sources(self):
  cases=('source /etc/vim/vimrc.local\n',
         'if filereadable("/etc/vim/other")\n source /etc/vim/vimrc.local\nendif\n',
         'if !filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local\nendif\n',
         'if filereadable("/etc/vim/vimrc.local")\nendif\nsource /etc/vim/vimrc.local\n',
         'if filereadable("/etc/vim/vimrc.local")\nelse\n source /etc/vim/vimrc.local\nendif\n',
         'if filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local | endif | source /etc/vim/vimrc.local\n',
         'if filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local\nendif\ninclude /opt/config\n')
  for content in cases:
   with self.subTest(content=content),tempfile.TemporaryDirectory() as d:
    warnings=self.collect_config(pathlib.Path(d),'etc/vim/vimrc',content)
    self.assertTrue(warnings)

 def test_broken_symlink_optional_vim_source_still_warns(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);target=root/'etc/vim/vimrc.local';target.parent.mkdir(parents=True);target.symlink_to('missing')
   warnings=self.collect_config(root,'etc/vim/vimrc','if filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local\nendif\n',paths=['/etc/vim/vimrc'])
   self.assertTrue(any('/etc/vim/vimrc.local' in warning for warning in warnings))

 def test_optional_vim_source_with_failed_metadata_lookup_still_warns(self):
  from unittest.mock import patch
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);real_lstat=pathlib.Path.lstat
   def lstat(path,*args,**kwargs):
    if path==root/'etc/vim/vimrc.local':raise PermissionError('metadata denied')
    return real_lstat(path,*args,**kwargs)
   with patch.object(pathlib.Path,'lstat',lstat):
    warnings=self.collect_config(root,'etc/vim/vimrc','if filereadable("/etc/vim/vimrc.local")\n source /etc/vim/vimrc.local\nendif\n')
   self.assertTrue(any('/etc/vim/vimrc.local' in warning for warning in warnings))

class DurableRestoreTests(unittest.TestCase):
 def test_rollback_is_synced_before_first_target_replace(self):
  from unittest.mock import patch
  import os,stat
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'etc').mkdir();target=root/'etc/test';target.write_bytes(b'old')
   rollback_synced=[];directory_synced=[];real_sync=helper.os.fsync;real_replace=helper.os.replace
   def sync(fd):
    if stat.S_ISDIR(os.fstat(fd).st_mode):directory_synced.append(True)
    else:
     try:
      path=os.readlink('/proc/self/fd/'+str(fd))
     except OSError:path=''
     rollback_synced.append(path)
    return real_sync(fd)
   def replace(src,dest):
    self.assertGreaterEqual(len(rollback_synced),3,'rollback and its metadata were not flushed')
    self.assertTrue(directory_synced,'rollback directories were not flushed')
    return real_replace(src,dest)
   with patch.object(helper.os,'fsync',side_effect=sync),patch.object(helper.os,'replace',side_effect=replace):
    helper.apply_files(root,[{'path':'etc/test','before_sha':helper.digest(b'old'),'content':'bmV3','mode':420,'uid':0,'gid':0}],test_mode=True)
   self.assertEqual(target.read_bytes(),b'new')
