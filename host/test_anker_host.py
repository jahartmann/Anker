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
